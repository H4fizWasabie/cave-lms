package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
)

// FakeWriter is an AI writer that records what it was asked. It gives the queued replies first, then for each
// request writes the Count questions asked for: "AI question 1" onward, each with a right and a wrong option.
type FakeWriter struct {
	mu       sync.Mutex
	requests []aiwriter.Request
	replies  []FakeReply
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
