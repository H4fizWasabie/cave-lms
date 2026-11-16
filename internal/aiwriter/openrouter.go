package aiwriter

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"

// maxReplyTokens caps one reply: about 50 Multiple-choice questions, the most one request asks for.
const maxReplyTokens = 16000

// maxMarkTokens caps one mark's reply: a number and a one-line reason, with room for a model that reasons first.
const maxMarkTokens = 2000

//go:embed write_questions.txt
var writeQuestionsInstructions string

//go:embed mark_answer.txt
var markAnswerInstructions string

//go:embed write_lesson.txt
var writeLessonInstructions string

// maxLessonTokens caps one Lesson's reply: 900 words of Malay Markdown, with room for a model that reasons first.
const maxLessonTokens = 6000

// OpenRouter writes through OpenRouter's chat API (https://openrouter.ai/docs/api-reference/chat-completion), with
// structured JSON output and no tools.
type OpenRouter struct {
	APIKey    string
	Model     string // writes and marks Exam questions and writes Lessons; empty turns those off
	DSKPModel string // reads DSKPs; empty turns that off
	WebSearch bool   // OpenRouter's web search plugin
	// Endpoint defaults to OpenRouter's chat completions URL.
	Endpoint string
	// HTTP defaults to a client that gives up after a minute.
	HTTP *http.Client
}

var openRouterClient = &http.Client{Timeout: 60 * time.Second}

var languageNames = map[string]string{"ms": "Malay", "en": "English"}

// questionsSchema is the reply's JSON schema for Multiple-choice questions. Counts are checked after, as strict
// schemas cannot hold them.
var questionsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"questions": {"type": "array", "items": {
		"type": "object",
		"properties": {
			"prompt": {"type": "string"},
			"options": {"type": "array", "items": {
				"type": "object",
				"properties": {"text": {"type": "string"}, "correct": {"type": "boolean"}},
				"required": ["text", "correct"], "additionalProperties": false}}},
		"required": ["prompt", "options"], "additionalProperties": false}}},
	"required": ["questions"], "additionalProperties": false}`)

// writtenSchema is the reply's JSON schema for Written questions.
var writtenSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"questions": {"type": "array", "items": {
		"type": "object",
		"properties": {"prompt": {"type": "string"}, "model_answer": {"type": "string"}},
		"required": ["prompt", "model_answer"], "additionalProperties": false}}},
	"required": ["questions"], "additionalProperties": false}`)

func (o OpenRouter) WriteQuestions(ctx context.Context, r Request) ([]Question, error) {
	if o.Model == "" {
		return nil, ErrNotConfigured
	}
	schema := questionsSchema
	if r.Format == "written" {
		schema = writtenSchema
	}
	user := fmt.Sprintf("Topic: %s\nFormat: %s\nLanguage: %s\nCount: %d\nDifficulty: %s", r.Topic, r.Format, languageNames[r.Language], r.Count, r.Difficulty)
	if r.KeyPoints != "" {
		user += "\nKey points:\n" + r.KeyPoints
	}
	if len(r.Avoid) > 0 {
		user += "\nExisting questions on this topic, not to repeat or closely rephrase:\n- " + strings.Join(r.Avoid, "\n- ")
	}
	req := map[string]any{
		"model":      o.Model,
		"max_tokens": maxReplyTokens,
		"messages": []map[string]string{
			{"role": "system", "content": writeQuestionsInstructions},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "questions", "strict": true, "schema": schema}},
	}
	if o.WebSearch {
		req["plugins"] = []map[string]string{{"id": "web"}}
	}
	var out struct {
		Questions []Question `json:"questions"`
	}
	if err := o.chat(ctx, req, &out); err != nil {
		return nil, err
	}
	for i, q := range out.Questions {
		out.Questions[i].Prompt = stripCitations(q.Prompt)
		out.Questions[i].ModelAnswer = stripCitations(q.ModelAnswer)
		for j, op := range q.Options {
			out.Questions[i].Options[j].Text = stripCitations(op.Text)
		}
	}
	return out.Questions, nil
}

// citation is a markdown link with an http(s) URL, as the web search plugin appends to what the model writes, with
// the space before it and optionally wrapped in parentheses. Written questions never hold a link.
var citation = regexp.MustCompile(`\s*\(\[[^\]]*\]\(https?://[^)\s]*\)\)|\s*\[[^\]]*\]\(https?://[^)\s]*\)`)

// stripCitations removes every citation from s.
func stripCitations(s string) string {
	return strings.TrimSpace(citation.ReplaceAllString(s, ""))
}

// markSchema is a mark's JSON schema. The range is checked after, as strict schemas cannot hold it.
var markSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"mark": {"type": "integer"}, "reason": {"type": "string"}},
	"required": ["mark", "reason"], "additionalProperties": false}`)

// MarkAnswer marks without web search: the model answer is what the answer is marked against.
func (o OpenRouter) MarkAnswer(ctx context.Context, r MarkRequest) (Marking, error) {
	if o.Model == "" {
		return Marking{}, ErrNotConfigured
	}
	req := map[string]any{
		"model":      o.Model,
		"max_tokens": maxMarkTokens,
		"messages": []map[string]string{
			{"role": "system", "content": markAnswerInstructions},
			{"role": "user", "content": fmt.Sprintf("Question: %s\nModel answer: %s\nAnswer: %s", r.Question, r.ModelAnswer, r.Answer)},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "mark", "strict": true, "schema": markSchema}},
	}
	var m Marking
	err := o.chat(ctx, req, &m)
	return m, err
}

// lessonSchema is a Lesson's JSON schema. Its length is checked after.
var lessonSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"markdown": {"type": "string"}},
	"required": ["markdown"], "additionalProperties": false}`)

// WriteLesson writes with web search when it is on, for current material, as WriteQuestions does.
func (o OpenRouter) WriteLesson(ctx context.Context, r LessonRequest) (string, error) {
	if o.Model == "" {
		return "", ErrNotConfigured
	}
	user := fmt.Sprintf("Course: %s\nLesson: %s\nLanguage: %s\nObjectives:\n- %s", r.CourseTitle, r.Title, languageNames[r.Language], strings.Join(r.Objectives, "\n- "))
	if len(r.OtherTitles) > 0 {
		user += "\nOther Lessons in the Course:\n- " + strings.Join(r.OtherTitles, "\n- ")
	}
	req := map[string]any{
		"model":      o.Model,
		"max_tokens": maxLessonTokens,
		"messages": []map[string]string{
			{"role": "system", "content": writeLessonInstructions},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "lesson", "strict": true, "schema": lessonSchema}},
	}
	if o.WebSearch {
		req["plugins"] = []map[string]string{{"id": "web"}}
	}
	var out struct {
		Markdown string `json:"markdown"`
	}
	err := o.chat(ctx, req, &out)
	return out.Markdown, err
}

//go:embed read_dskp.txt
var readDSKPInstructions string

// maxDSKPTokens caps one DSKP's reply: a DSKP holds hundreds of standards, with room for a model that reasons first.
const maxDSKPTokens = 60000

// dskpClient waits for a DSKP's reply as long as a model needs to read a long PDF.
var dskpClient = &http.Client{Timeout: 10 * time.Minute}

// dskpSchema is the JSON schema of a DSKP's standards. That an SK has an SP is checked after.
var dskpSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"standards": {"type": "array", "items": {
		"type": "object",
		"properties": {
			"grouping": {"type": "string"},
			"topic": {"type": "string"},
			"code": {"type": "string"},
			"text": {"type": "string"},
			"sps": {"type": "array", "items": {
				"type": "object",
				"properties": {"code": {"type": "string"}, "text": {"type": "string"}},
				"required": ["code", "text"], "additionalProperties": false}}},
		"required": ["grouping", "topic", "code", "text", "sps"], "additionalProperties": false}}},
	"required": ["standards"], "additionalProperties": false}`)

func (o OpenRouter) ReadsDSKP() bool { return o.DSKPModel != "" }

// ReadDSKP sends the PDF as a file content part (https://openrouter.ai/docs/guides/overview/multimodal/pdfs); with no
// file-parser engine named, OpenRouter reads it natively when the model can and otherwise with OCR. Web search is off:
// the standards are only what the PDF says.
func (o OpenRouter) ReadDSKP(ctx context.Context, r DSKPRequest) ([]SK, error) {
	if o.DSKPModel == "" {
		return nil, ErrNotConfigured
	}
	o.HTTP = dskpClient
	req := map[string]any{
		"model":      o.DSKPModel,
		"max_tokens": maxDSKPTokens,
		"messages": []map[string]any{
			{"role": "system", "content": readDSKPInstructions},
			{"role": "user", "content": []map[string]any{
				{"type": "text", "text": "Language: " + languageNames[r.Language]},
				{"type": "file", "file": map[string]string{"filename": "dskp.pdf", "file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(r.PDF)}},
			}},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "dskp", "strict": true, "schema": dskpSchema}},
	}
	var out struct {
		Standards []SK `json:"standards"`
	}
	if err := o.chat(ctx, req, &out); err != nil {
		return nil, err
	}
	return out.Standards, nil
}

// chat sends req and decodes the first reply's JSON content into v. Content that is not v's JSON is ErrBadShape.
func (o OpenRouter) chat(ctx context.Context, req map[string]any, v any) error {
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := o.post(ctx, req, &reply); err != nil {
		return err
	}
	if len(reply.Choices) == 0 || json.Unmarshal([]byte(reply.Choices[0].Message.Content), v) != nil {
		return ErrBadShape
	}
	return nil
}

// post sends body and decodes the reply into v. A refused key is ErrNotConfigured, no credits or a spent key
// limit ErrOverLimit, anything else that fails ErrUnavailable.
func (o OpenRouter) post(ctx context.Context, body, v any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := o.Endpoint
	if endpoint == "" {
		endpoint = openRouterEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := o.HTTP
	if client == nil {
		client = openRouterClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode/100 == 2 {
		if json.Unmarshal(reply, v) != nil {
			return ErrBadShape
		}
		return nil
	}
	var e struct {
		Error struct {
			Metadata struct {
				LimitSource string `json:"limit_source"`
			} `json:"metadata"`
		} `json:"error"`
	}
	_ = json.Unmarshal(reply, &e)
	kind := ErrUnavailable
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		kind = ErrNotConfigured
	case resp.StatusCode == http.StatusPaymentRequired, resp.StatusCode == http.StatusForbidden && e.Error.Metadata.LimitSource != "":
		kind = ErrOverLimit
	}
	return fmt.Errorf("%w: openrouter: %s: %s", kind, resp.Status, bytes.TrimSpace(reply[:min(len(reply), 4096)]))
}
