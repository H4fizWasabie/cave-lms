package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
)

// FakeWriter is an AI writer that records what it was asked. It gives the queued replies first, then for each
// request writes the Count questions asked for: "AI question 1" onward, each with a right and a wrong option. It
// marks each answer with the queued marks first, then FakeMarking, or fails every mark while FailMarks is set.
type FakeWriter struct {
	mu           sync.Mutex
	requests     []aiwriter.Request
	replies      []FakeReply
	markRequests []aiwriter.MarkRequest
	marks        []FakeMark
	markErr      error
}

// FakeMarking is the fake writer's mark when none is queued.
var FakeMarking = aiwriter.Marking{Mark: 7, Reason: "Gets the main idea."}

// FakeMark is one queued mark: Marking as it is, or Err.
type FakeMark struct {
	Marking aiwriter.Marking
	Err     error
}

func (f *FakeWriter) MarkAnswer(ctx context.Context, r aiwriter.MarkRequest) (aiwriter.Marking, error) {
	if err := ctx.Err(); err != nil {
		return aiwriter.Marking{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markRequests = append(f.markRequests, r)
	if len(f.marks) > 0 {
		m := f.marks[0]
		f.marks = f.marks[1:]
		return m.Marking, m.Err
	}
	if f.markErr != nil {
		return aiwriter.Marking{}, f.markErr
	}
	return FakeMarking, nil
}

// ReplyMarks queues marks for the next mark requests.
func (f *FakeWriter) ReplyMarks(marks ...FakeMark) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks = append(f.marks, marks...)
}

// FailMarks makes every later mark without a queued one fail with err; nil marks again.
func (f *FakeWriter) FailMarks(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markErr = err
}

// MarkRequests lists the answers the writer was asked to mark, oldest first.
func (f *FakeWriter) MarkRequests() []aiwriter.MarkRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]aiwriter.MarkRequest(nil), f.markRequests...)
}

// FakeReply is one queued reply: Questions as they are, or Err.
type FakeReply struct {
	Questions []aiwriter.Question
	Err       error
}

func (f *FakeWriter) WriteQuestions(ctx context.Context, r aiwriter.Request) ([]aiwriter.Question, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if len(f.replies) > 0 {
		reply := f.replies[0]
		f.replies = f.replies[1:]
		return reply.Questions, reply.Err
	}
	qs := make([]aiwriter.Question, r.Count)
	for i := range qs {
		qs[i] = aiwriter.Question{Prompt: fmt.Sprintf("AI question %d", i+1),
			Options: []aiwriter.Option{{Text: "AI right", Correct: true}, {Text: "AI wrong"}}}
	}
	return qs, nil
}

// Reply queues replies for the next requests.
func (f *FakeWriter) Reply(replies ...FakeReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, replies...)
}

// Requests lists what the writer was asked, oldest first.
func (f *FakeWriter) Requests() []aiwriter.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]aiwriter.Request(nil), f.requests...)
}
