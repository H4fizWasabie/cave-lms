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

var sains = DSKPRequest{PDF: []byte("%PDF-1.4 fake"), Language: "ms"}

func TestReadDSKPAsksOnceMoreWhenNoSKHasAnSP(t *testing.T) {
	good := []SK{{Grouping: "Tema 1", Topic: "Sains", Code: "1.1", Text: "Memahami", SPs: []SP{{Code: "1.1.1", Text: "Menyatakan"}}}}
	for _, c := range []struct {
		replies [][]SK
		want    []SK
		err     error
	}{
		{[][]SK{nil, good}, good, nil},
		{[][]SK{{{Code: "1.1", Text: "Memahami"}}, good}, good, nil},
		{[][]SK{{{Code: "1.1", Text: "x", SPs: []SP{{Code: "1.1.1"}}}}, good}, good, nil}, // a blank SP text
		{[][]SK{nil, nil}, nil, ErrBadShape},
	} {
		calls := 0
		w := dskpFunc(func(DSKPRequest) ([]SK, error) { calls++; return c.replies[calls-1], nil })
		got, err := ReadDSKP(context.Background(), w, sains)
		if len(got) != len(c.want) || !errors.Is(err, c.err) || calls != 2 {
			t.Errorf("ReadDSKP() = %v, %v after %d calls, want %v, %v after 2", got, err, calls, c.want, c.err)
		}
	}
}

type dskpFunc func(DSKPRequest) ([]SK, error)

func (f dskpFunc) WriteQuestions(context.Context, Request) ([]Question, error) { return nil, nil }
func (f dskpFunc) MarkAnswer(context.Context, MarkRequest) (Marking, error)    { return Marking{}, nil }
func (f dskpFunc) WriteLesson(context.Context, LessonRequest) (string, error)  { return "", nil }
func (f dskpFunc) ReadsDSKP() bool                                             { return true }
func (f dskpFunc) ReadDSKP(_ context.Context, r DSKPRequest) ([]SK, error)     { return f(r) }

func TestOpenRouterSendsTheDSKPAsAPDFFilePartToTheDSKPModelAndReadsTheStandards(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"standards\":[{\"grouping\":\"Tema 1\",\"topic\":\"Sains\",\"code\":\"1.1\",\"text\":\"Memahami\",\"sps\":[{\"code\":\"1.1.1\",\"text\":\"Menyatakan\"}]}]}"}}]}`))
	}))
	defer server.Close()

	w := OpenRouter{APIKey: "sk-or-test", Model: "exam-model", DSKPModel: "dskp-model", Endpoint: server.URL}
	sks, err := w.ReadDSKP(context.Background(), sains)
	if err != nil || len(sks) != 1 || sks[0].Topic != "Sains" || sks[0].SPs[0].Code != "1.1.1" {
		t.Fatalf("ReadDSKP() = %#v, %v", sks, err)
	}
	if got.Model != "dskp-model" || len(got.Messages) != 2 {
		t.Fatalf("model = %q, messages = %d", got.Model, len(got.Messages))
	}
	var parts []struct {
		Type string `json:"type"`
		File struct {
			Filename string `json:"filename"`
			FileData string `json:"file_data"`
		} `json:"file"`
	}
	if err := json.Unmarshal(got.Messages[1].Content, &parts); err != nil || len(parts) != 2 || parts[1].Type != "file" ||
		parts[1].File.FileData != "data:application/pdf;base64,JVBERi0xLjQgZmFrZQ==" || !strings.HasSuffix(parts[1].File.Filename, ".pdf") {
		t.Fatalf("content = %s (%v)", got.Messages[1].Content, err)
	}
}

func TestEachOperationNeedsItsOwnModel(t *testing.T) {
	both := New("sk-or-test", "m", "d", true)
	if !both.ReadsDSKP() {
		t.Error("with both models, DSKP reading is off")
	}
	onlyExams, onlyDSKP := New("sk-or-test", "m", "", true), New("sk-or-test", "", "d", true)
	if onlyExams.ReadsDSKP() || !onlyDSKP.ReadsDSKP() {
		t.Errorf("ReadsDSKP: exams-only %v, dskp-only %v", onlyExams.ReadsDSKP(), onlyDSKP.ReadsDSKP())
	}
	if _, err := onlyExams.ReadDSKP(context.Background(), sains); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("ReadDSKP() with no DSKP model = %v, want ErrNotConfigured", err)
	}
	if _, err := onlyDSKP.MarkAnswer(context.Background(), MarkRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("MarkAnswer() with no Exam model = %v, want ErrNotConfigured", err)
	}
	if New("", "m", "d", true).ReadsDSKP() || (Unconfigured{}).ReadsDSKP() {
		t.Error("with no key, DSKP reading is on")
	}
}
