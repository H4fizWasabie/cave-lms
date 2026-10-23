// Package aiwriter asks an AI to write Exam questions and mark Written answers. Only the topic, format, language and
// count go out for questions, and only the question, model answer and answer for a mark.
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
	Topic    string
	Format   string // "multiple-choice"
	Language string // "ms" or "en"
	Count    int
}

// Question is one question the AI wrote.
type Question struct {
	Prompt  string   `json:"prompt"`
	Options []Option `json:"options"`
}

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

type Writer interface {
	// WriteQuestions writes r.Count questions. A reply that cannot be read is ErrBadShape.
	WriteQuestions(context.Context, Request) ([]Question, error)
	// MarkAnswer marks r's answer against its model answer. A reply that cannot be read is ErrBadShape.
	MarkAnswer(context.Context, MarkRequest) (Marking, error)
}

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
	var err error
	for range 2 {
		var m Marking
		if m, err = w.MarkAnswer(ctx, r); err == nil && (m.Mark < 0 || m.Mark > MaxMark || strings.TrimSpace(m.Reason) == "") {
			err = ErrBadShape
		}
		if !errors.Is(err, ErrBadShape) {
			return m, err
		}
	}
	return Marking{}, err
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
	var err error
	for range 2 {
		var qs []Question
		if qs, err = w.WriteQuestions(ctx, r); err == nil && !wellShaped(r, qs) {
			err = ErrBadShape
		}
		if !errors.Is(err, ErrBadShape) {
			return qs, err
		}
	}
	return nil, err
}

// wellShaped reports whether qs are r.Count Multiple-choice questions, each with a prompt and 2 to 5 options with
// text, exactly one of them correct.
func wellShaped(r Request, qs []Question) bool {
	if len(qs) != r.Count {
		return false
	}
	for _, q := range qs {
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
