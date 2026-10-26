package aiwriter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var excel = Request{Topic: "Excel pivot tables", Format: "multiple-choice", Language: "ms", Count: 1, Difficulty: "hard"}

func TestAnUnsetOpenRouterKeyOrModelLeavesTheWriterNotConfigured(t *testing.T) {
	for _, w := range []Writer{New("", "openai/gpt-4o-mini", true), New("sk-or-test", "", true)} {
		if _, err := w.WriteQuestions(context.Background(), excel); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("WriteQuestions() error = %v, want ErrNotConfigured", err)
		}
	}
}

func TestOpenRouterSendsTheInstructionsAndOnlyTheRequestAndReadsTheQuestions(t *testing.T) {
	var got struct {
		auth string
		body struct {
			Model     string              `json:"model"`
			MaxTokens int                 `json:"max_tokens"`
			Messages  []map[string]string `json:"messages"`
			Format    struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Plugins []map[string]string `json:"plugins"`
			Tools   any                 `json:"tools"`
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"questions\":[{\"prompt\":\"Apakah jadual pangsi?\",\"options\":[{\"text\":\"Ringkasan data\",\"correct\":true},{\"text\":\"Carta\",\"correct\":false}]}]}"}}]}`))
	}))
	defer server.Close()

	w := OpenRouter{APIKey: "sk-or-test", Model: "openai/gpt-4o-mini", WebSearch: true, Endpoint: server.URL}
	qs, err := w.WriteQuestions(context.Background(), excel)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].Prompt != "Apakah jadual pangsi?" || len(qs[0].Options) != 2 || !qs[0].Options[0].Correct || qs[0].Options[1].Correct {
		t.Fatalf("questions = %#v", qs)
	}
	b := got.body
	if got.auth != "Bearer sk-or-test" || b.Model != "openai/gpt-4o-mini" || b.MaxTokens != maxReplyTokens || b.Format.Type != "json_schema" || b.Tools != nil {
		t.Fatalf("auth = %q, body = %#v", got.auth, b)
	}
	if len(b.Plugins) != 1 || b.Plugins[0]["id"] != "web" {
		t.Fatalf("plugins = %v, want web search", b.Plugins)
	}
	if len(b.Messages) != 2 || b.Messages[0]["content"] != writeQuestionsInstructions || !strings.Contains(writeQuestionsInstructions, "exactly one of them correct") ||
		b.Messages[1]["content"] != "Topic: Excel pivot tables\nFormat: multiple-choice\nLanguage: Malay\nCount: 1\nDifficulty: hard" {
		t.Fatalf("messages = %q", b.Messages)
	}

	r := excel
	r.KeyPoints, r.Avoid = "Grouping\nFilters", []string{"Apakah jadual pangsi?", "Bagaimana menapis?"}
	if _, err := w.WriteQuestions(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if want := "Topic: Excel pivot tables\nFormat: multiple-choice\nLanguage: Malay\nCount: 1\nDifficulty: hard" +
		"\nKey points:\nGrouping\nFilters" +
		"\nExisting questions on this topic, not to repeat or closely rephrase:\n- Apakah jadual pangsi?\n- Bagaimana menapis?"; got.body.Messages[1]["content"] != want {
		t.Fatalf("user message = %q, want %q", got.body.Messages[1]["content"], want)
	}
	if !strings.Contains(writeQuestionsInstructions, "key points") || !strings.Contains(writeQuestionsInstructions, "closely rephrase") {
		t.Fatalf("the instructions don't explain key points and existing questions: %s", writeQuestionsInstructions)
	}

	w.WebSearch, got.body.Plugins = false, nil
	if _, err := w.WriteQuestions(context.Background(), excel); err != nil || got.body.Plugins != nil {
		t.Fatalf("with web search off: plugins = %v, err = %v", got.body.Plugins, err)
	}
}

func TestOpenRouterTellsAFailureFromAnUnreadableReply(t *testing.T) {
	for _, c := range []struct {
		status int
		reply  string
		want   error
	}{
		{http.StatusOK, `{"choices":[{"message":{"content":"Here are your questions!"}}]}`, ErrBadShape},
		{http.StatusOK, `{"choices":[]}`, ErrBadShape},
		{http.StatusUnauthorized, `{"error":{"code":401,"message":"No auth credentials found"}}`, ErrNotConfigured},
		{http.StatusPaymentRequired, `{"error":{"code":402,"message":"Insufficient credits"}}`, ErrOverLimit},
		{http.StatusForbidden, `{"error":{"code":403,"message":"Key limit exceeded","metadata":{"limit_source":"openrouter_key_limit"}}}`, ErrOverLimit},
		{http.StatusForbidden, `{"error":{"code":403,"message":"Flagged","metadata":{"reasons":["x"]}}}`, ErrUnavailable},
		{http.StatusTooManyRequests, `{"error":{"code":429,"message":"Rate limited"}}`, ErrUnavailable},
		{http.StatusBadGateway, `{"error":{"code":502,"message":"Model down"}}`, ErrUnavailable},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.reply))
		}))
		_, err := OpenRouter{APIKey: "sk-or-test", Model: "m", Endpoint: server.URL}.WriteQuestions(context.Background(), excel)
		server.Close()
		if !errors.Is(err, c.want) {
			t.Errorf("%d %s: error = %v, want %v", c.status, c.reply, err, c.want)
		}
	}
}

var markExplain = MarkRequest{Question: "Explain :=", ModelAnswer: "It declares and assigns.", Answer: "It makes a variable."}

func TestOpenRouterMarksAnAnswerSendingOnlyTheQuestionModelAnswerAndAnswer(t *testing.T) {
	var body struct {
		MaxTokens int                 `json:"max_tokens"`
		Messages  []map[string]string `json:"messages"`
		Format    struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Plugins any `json:"plugins"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"mark\":7,\"reason\":\"Right idea, misses the assignment.\"}"}}]}`))
	}))
	defer server.Close()

	got, err := OpenRouter{APIKey: "sk-or-test", Model: "m", WebSearch: true, Endpoint: server.URL}.MarkAnswer(context.Background(), markExplain)
	if err != nil || got != (Marking{Mark: 7, Reason: "Right idea, misses the assignment."}) {
		t.Fatalf("MarkAnswer() = %#v, %v", got, err)
	}
	if body.MaxTokens != maxMarkTokens || body.Format.Type != "json_schema" || body.Plugins != nil {
		t.Fatalf("body = %#v, want a capped structured reply and no web search", body)
	}
	if len(body.Messages) != 2 || body.Messages[0]["content"] != markAnswerInstructions || !strings.Contains(markAnswerInstructions, "0 to 10") ||
		body.Messages[1]["content"] != "Question: Explain :=\nModel answer: It declares and assigns.\nAnswer: It makes a variable." {
		t.Fatalf("messages = %q", body.Messages)
	}
}

func TestMarkAsksOnceMoreWhenTheMarkIsTheWrongShape(t *testing.T) {
	for _, c := range []struct {
		replies []string
		want    Marking
		err     error
	}{
		{[]string{`{"mark":11,"reason":"x"}`, `{"mark":10,"reason":"Full marks."}`}, Marking{Mark: 10, Reason: "Full marks."}, nil},
		{[]string{`{"mark":7.5,"reason":"x"}`, `{"mark":-1,"reason":"x"}`}, Marking{}, ErrBadShape},
		{[]string{`{"mark":5,"reason":" "}`, `{"mark":5}`}, Marking{}, ErrBadShape},
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			content, _ := json.Marshal(c.replies[calls])
			calls++
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + string(content) + `}}]}`))
		}))
		got, err := Mark(context.Background(), OpenRouter{APIKey: "sk-or-test", Model: "m", Endpoint: server.URL}, markExplain)
		server.Close()
		if got != c.want || !errors.Is(err, c.err) || calls != 2 {
			t.Errorf("%v: Mark() = %#v, %v after %d calls, want %#v, %v after 2", c.replies, got, err, calls, c.want, c.err)
		}
	}
	if _, err := New("", "", false).MarkAnswer(context.Background(), markExplain); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured MarkAnswer() error = %v", err)
	}
}

func TestOpenRouterAsksForAndReadsWrittenQuestionsWithAModelAnswer(t *testing.T) {
	var sent struct {
		Messages []map[string]string `json:"messages"`
		Format   struct {
			JSONSchema struct {
				Schema struct {
					Properties struct {
						Questions struct {
							Items struct {
								Required []string `json:"required"`
							} `json:"items"`
						} `json:"questions"`
					} `json:"properties"`
				} `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"questions\":[{\"prompt\":\"Terangkan jadual pangsi.\",\"model_answer\":\"Ia meringkaskan data.\"}]}"}}]}`))
	}))
	defer server.Close()

	written := Request{Topic: "Excel pivot tables", Format: "written", Language: "ms", Count: 1, Difficulty: "hard"}
	qs, err := OpenRouter{APIKey: "sk-or-test", Model: "m", Endpoint: server.URL}.WriteQuestions(context.Background(), written)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].Prompt != "Terangkan jadual pangsi." || qs[0].ModelAnswer != "Ia meringkaskan data." || qs[0].Options != nil {
		t.Fatalf("questions = %#v", qs)
	}
	if req := sent.Format.JSONSchema.Schema.Properties.Questions.Items.Required; strings.Join(req, ",") != "prompt,model_answer" {
		t.Fatalf("the reply schema requires %v, want prompt and model_answer", req)
	}
	if len(sent.Messages) != 2 || !strings.Contains(sent.Messages[0]["content"], `Format "written"`) ||
		sent.Messages[1]["content"] != "Topic: Excel pivot tables\nFormat: written\nLanguage: Malay\nCount: 1\nDifficulty: hard" {
		t.Fatalf("messages = %q", sent.Messages)
	}
}

var pivotLesson = LessonRequest{CourseTitle: "Excel for the office", Title: "Pivot tables", Language: "ms",
	Objectives: []string{"Build a pivot table.", "Filter a pivot table."}, OtherTitles: []string{"Formulas", "Charts"}}

// lessonText is a reply of n words.
func lessonText(n int) string { return strings.TrimSpace(strings.Repeat("perkataan ", n)) }

func TestOpenRouterWritesALessonSendingOnlyTheCourseTitleLessonTitleObjectivesOtherTitlesAndLanguage(t *testing.T) {
	var body struct {
		MaxTokens int                 `json:"max_tokens"`
		Messages  []map[string]string `json:"messages"`
		Format    struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		content, _ := json.Marshal(map[string]string{"markdown": lessonText(600)})
		reply, _ := json.Marshal(string(content))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + string(reply) + `}}]}`))
	}))
	defer server.Close()

	got, err := Lesson(context.Background(), OpenRouter{APIKey: "sk-or-test", Model: "m", Endpoint: server.URL}, pivotLesson)
	if err != nil || got != lessonText(600) {
		t.Fatalf("Lesson() = %d words, %v", len(strings.Fields(got)), err)
	}
	if body.MaxTokens != maxLessonTokens || body.Format.Type != "json_schema" {
		t.Fatalf("body = %#v, want a capped structured reply", body)
	}
	want := "Course: Excel for the office\nLesson: Pivot tables\nLanguage: Malay\nObjectives:\n- Build a pivot table.\n- Filter a pivot table." +
		"\nOther Lessons in the Course:\n- Formulas\n- Charts"
	if len(body.Messages) != 2 || body.Messages[0]["content"] != writeLessonInstructions || body.Messages[1]["content"] != want {
		t.Fatalf("messages = %q, want the instructions then %q", body.Messages, want)
	}
	for _, s := range []string{"one section per Objective", "500 to 900 words", "Other Lessons"} {
		if !strings.Contains(writeLessonInstructions, s) {
			t.Errorf("the instructions don't say %q: %s", s, writeLessonInstructions)
		}
	}
}

func TestLessonAsksOnceMoreWhenTheTextIsEmptyOrBadlySized(t *testing.T) {
	for _, c := range []struct {
		replies []string
		want    string
		err     error
	}{
		{[]string{" ", lessonText(500)}, lessonText(500), nil},
		{[]string{lessonText(20), lessonText(5000)}, "", ErrBadShape},
		{[]string{strings.Repeat("x", 20001), ""}, "", ErrBadShape},
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			content, _ := json.Marshal(map[string]string{"markdown": c.replies[calls]})
			reply, _ := json.Marshal(string(content))
			calls++
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + string(reply) + `}}]}`))
		}))
		got, err := Lesson(context.Background(), OpenRouter{APIKey: "sk-or-test", Model: "m", Endpoint: server.URL}, pivotLesson)
		server.Close()
		if got != c.want || !errors.Is(err, c.err) || calls != 2 {
			t.Errorf("Lesson() = %d words, %v after %d calls, want %d words, %v after 2", len(strings.Fields(got)), err, calls, len(strings.Fields(c.want)), c.err)
		}
	}
	if _, err := Lesson(context.Background(), New("", "", false), pivotLesson); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured Lesson() error = %v", err)
	}
}
