// Package aiwriter asks an AI to write Exam questions and mark Written answers. Only the topic, format, language,
// count, Difficulty, the Admin's key points and the Question bank's existing prompts on the topic go out for
// questions, and only the question, model answer and answer for a mark. No Learner data goes out for questions.
package aiwriter

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrNotConfigured = errors.New("the AI writer is not configured")
	ErrUnavailable   = errors.New("the AI service is unavailable")
	ErrOverLimit     = errors.New("the AI key is over its spending limit")
	ErrBadShape      = errors.New("the AI's reply is not in the required shape")
)

// Request is everything the AI is told: nothing about a Course or a Learner.
type Request struct {
	Topic      string
	Format     string // "multiple-choice" or "written"
	Language   string // "ms" or "en"
	Count      int
	Difficulty string   // "easy", "medium" or "hard"
	KeyPoints  string   // what the questions should keep to, empty for the whole topic
	Avoid      []string // existing prompts on the topic, not to be repeated or closely rephrased
}

// Question is one question the AI wrote: a Multiple-choice one has Options, a Written one a ModelAnswer.
type Question struct {
	Prompt      string   `json:"prompt"`
	Options     []Option `json:"options,omitempty"`
	ModelAnswer string   `json:"model_answer,omitempty"`
}

// Option is one option of a Multiple-choice question.
type Option struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

// MarkRequest is everything the AI is told to mark a Written answer: nothing about the Learner (ADR 0009).
type MarkRequest struct {
	Question, ModelAnswer, Answer string
}

// Marking is the AI's mark for a Written answer, 0 to MaxMark, with a one-line reason.
type Marking struct {
	Mark   int    `json:"mark"`
	Reason string `json:"reason"`
}

// MaxMark is what a Written answer is marked out of.
const MaxMark = 10

// Writer is an AI that writes Exam questions and marks Written answers. Callers go through Questions and Mark,
// which check the reply's shape.
type Writer interface {
	// WriteQuestions writes r.Count questions. A reply that cannot be read is ErrBadShape.
	WriteQuestions(context.Context, Request) ([]Question, error)
	// MarkAnswer marks r's answer against its model answer. A reply that cannot be read is ErrBadShape.
	MarkAnswer(context.Context, MarkRequest) (Marking, error)
}

// Unconfigured is the Writer when no AI is set up: every call is ErrNotConfigured.
type Unconfigured struct{}

func (Unconfigured) WriteQuestions(context.Context, Request) ([]Question, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) MarkAnswer(context.Context, MarkRequest) (Marking, error) {
	return Marking{}, ErrNotConfigured
}

// Mark asks w to mark r, asking once more when the mark is not a whole number 0 to MaxMark with a reason. A second
// wrong shape is ErrBadShape.
func Mark(ctx context.Context, w Writer, r MarkRequest) (Marking, error) {
	return askTwice(func() (Marking, error) { return w.MarkAnswer(ctx, r) }, func(m Marking) bool {
		return m.Mark >= 0 && m.Mark <= MaxMark && strings.TrimSpace(m.Reason) != ""
	})
}

// New is the OpenRouter writer, or Unconfigured when the key or model is unset.
func New(apiKey, model string, webSearch bool) Writer {
	if apiKey == "" || model == "" {
		return Unconfigured{}
	}
	return OpenRouter{APIKey: apiKey, Model: model, WebSearch: webSearch}
}

// Questions asks w for r's questions, asking once more when the reply is the wrong shape. A second wrong shape
// is ErrBadShape, so nothing invalid is ever returned.
func Questions(ctx context.Context, w Writer, r Request) ([]Question, error) {
	return askTwice(func() ([]Question, error) { return w.WriteQuestions(ctx, r) }, func(qs []Question) bool {
		return wellShaped(r, qs)
	})
}

// askTwice calls ask, and once more when its reply is ErrBadShape or fails ok. A second wrong shape is ErrBadShape,
// with the zero reply.
func askTwice[T any](ask func() (T, error), ok func(T) bool) (T, error) {
	var err error
	for range 2 {
		var v T
		if v, err = ask(); err == nil && !ok(v) {
			err = ErrBadShape
		}
		if !errors.Is(err, ErrBadShape) {
			return v, err
		}
	}
	var zero T
	return zero, err
}

// wellShaped reports whether qs are r.Count questions in r's format, each with a prompt and, when Written, a model
// answer and no options, or else 2 to 5 options with text, exactly one of them correct.
func wellShaped(r Request, qs []Question) bool {
	if len(qs) != r.Count {
		return false
	}
	for _, q := range qs {
		if r.Format == "written" {
			if strings.TrimSpace(q.Prompt) == "" || strings.TrimSpace(q.ModelAnswer) == "" || len(q.Options) > 0 {
				return false
			}
			continue
		}
		correct := 0
		for _, o := range q.Options {
			if strings.TrimSpace(o.Text) == "" {
				return false
			}
			if o.Correct {
				correct++
			}
		}
		if strings.TrimSpace(q.Prompt) == "" || len(q.Options) < 2 || len(q.Options) > 5 || correct != 1 {
			return false
		}
	}
	return true
}
