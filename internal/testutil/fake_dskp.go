package testutil

import (
	"context"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
)

// The DSKP half of FakeWriter: it reads each DSKP with the queued readings first, then FakeDSKP, and reads them
// all while DSKPs are not held.

// FakeDSKP is the fake writer's DSKP reading when none is queued: two SKs under one tema, the first with two SPs.
var FakeDSKP = []aiwriter.SK{
	{Grouping: "Tema 1: Kajian Saintifik", Topic: "Kemahiran Saintifik", Code: "1.1", Text: "Memahami kemahiran saintifik", SPs: []aiwriter.SP{
		{Code: "1.1.1", Text: "Mengenal pasti kemahiran proses sains"},
		{Code: "1.1.2", Text: "Menyatakan kemahiran manipulatif"},
	}},
	{Grouping: "Tema 1: Kajian Saintifik", Topic: "Penyiasatan", Code: "1.2", Text: "Menjalankan penyiasatan", SPs: []aiwriter.SP{
		{Code: "1.2.1", Text: "Membuat hipotesis"},
	}},
}

// FakeDSKPReply is one queued DSKP reading: SKs as they are, or Err.
type FakeDSKPReply struct {
	SKs []aiwriter.SK
	Err error
}

type fakeDSKP struct {
	off      bool
	requests []aiwriter.DSKPRequest
	replies  []FakeDSKPReply
	hold     chan struct{}
}

// ReadsDSKP is true unless DSKPOff was called.
func (f *FakeWriter) ReadsDSKP() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.dskp.off
}

// DSKPOff turns DSKP reading off, as an unset OPENROUTER_DSKP_MODEL does.
func (f *FakeWriter) DSKPOff() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dskp.off = true
}

func (f *FakeWriter) ReadDSKP(ctx context.Context, r aiwriter.DSKPRequest) ([]aiwriter.SK, error) {
	f.mu.Lock()
	f.dskp.requests = append(f.dskp.requests, r)
	hold := f.dskp.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.dskp.replies) > 0 {
		reply := f.dskp.replies[0]
		f.dskp.replies = f.dskp.replies[1:]
		return reply.SKs, reply.Err
	}
	return FakeDSKP, nil
}

// ReplyDSKPs queues readings for the next DSKP requests.
func (f *FakeWriter) ReplyDSKPs(replies ...FakeDSKPReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dskp.replies = append(f.dskp.replies, replies...)
}

// HoldDSKPs makes every later DSKP reading wait until the returned release is called, so a test can look at a DSKP
// while it is being read.
func (f *FakeWriter) HoldDSKPs() (release func()) {
	hold := make(chan struct{})
	f.mu.Lock()
	f.dskp.hold = hold
	f.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(hold) }) }
}

// DSKPRequests lists the DSKPs the writer was asked to read, oldest first.
func (f *FakeWriter) DSKPRequests() []aiwriter.DSKPRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]aiwriter.DSKPRequest(nil), f.dskp.requests...)
}
