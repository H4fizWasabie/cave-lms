package aiwriter

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// OpenRouter writes through OpenRouter's chat API (https://openrouter.ai/docs/api-reference/chat-completion), with
// structured JSON output and no tools.
type OpenRouter struct {
	APIKey    string
	Model     string
	WebSearch bool // OpenRouter's web search plugin
	// Endpoint defaults to OpenRouter's chat completions URL.
	Endpoint string
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
	schema := questionsSchema
	if r.Format == "written" {
		schema = writtenSchema
	}
	req := map[string]any{
		"model":      o.Model,
		"max_tokens": maxReplyTokens,
		"messages": []map[string]string{
			{"role": "system", "content": writeQuestionsInstructions},
			{"role": "user", "content": fmt.Sprintf("Topic: %s\nFormat: %s\nLanguage: %s\nCount: %d", r.Topic, r.Format, languageNames[r.Language], r.Count)},
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
	return out.Questions, nil
}

// markSchema is a mark's JSON schema. The range is checked after, as strict schemas cannot hold it.
var markSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"mark": {"type": "integer"}, "reason": {"type": "string"}},
	"required": ["mark", "reason"], "additionalProperties": false}`)

// MarkAnswer marks without web search: the model answer is what the answer is marked against.
func (o OpenRouter) MarkAnswer(ctx context.Context, r MarkRequest) (Marking, error) {
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
	resp, err := openRouterClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
