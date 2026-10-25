package app_test

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// chatStream is an open event stream of a Course's Chat; events carries each event's data.
type chatStream struct {
	t      *testing.T
	events chan string
}

// openStream opens Course id's Chat, or the Community Chat's, event stream as s, which must be allowed it; the stream closes with the test.
func (s *site) openStream(id string) *chatStream {
	s.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s.t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.server.URL+chatURL(id)+"/events", nil)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", "en")
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		s.t.Fatalf("the Chat stream answered %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	st := &chatStream{t: s.t, events: make(chan string, 100)}
	go func() {
		defer res.Body.Close()
		lines := bufio.NewScanner(res.Body)
		var data []string
		for lines.Scan() {
			line := lines.Text()
			if d, ok := strings.CutPrefix(line, "data: "); ok {
				data = append(data, d)
			} else if line == "" && data != nil {
				st.events <- strings.Join(data, "\n")
				data = nil
			}
		}
	}()
	return st
}

// next waits for the stream's next event.
func (st *chatStream) next() string {
	st.t.Helper()
	select {
	case e := <-st.events:
		return e
	case <-time.After(5 * time.Second):
		st.t.Fatal("no event on the Chat stream within 5 seconds")
		return ""
	}
}

func TestAnOpenChatStreamReceivesNewAndChangedMessages(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	stream := ada.openStream(id)
	adminStream := admin.openStream(id)

	mid := ben.say(id, "Live hello")
	e := stream.next()
	expectInOrder(t, e, `<li class="msg" id="m`+mid+`"`, "Ben", "Live hello")
	adminStream.next()

	ben.react(id, mid, laugh)
	expectInOrder(t, stream.next(), `id="m`+mid+`"`, shown(laugh, false, 1))
	adminStream.next()

	rid := ada.reply(id, mid, "Hi Ben")
	expectInOrder(t, stream.next(), `id="m`+rid+`"`, "Live hello", "Hi Ben")
	adminStream.next()

	admin.post("/admin/chat/"+mid+"/hide", nil)
	for _, e := range []string{stream.next(), stream.next()} { // the Message, and the reply quoting it
		if strings.Contains(e, "Live hello") {
			t.Fatalf("a Learner's stream carries a hidden Message's text: %s", e)
		}
	}
	e = adminStream.next()
	expectInOrder(t, e, `is-hidden" id="m`+mid+`"`, "Live hello")

	ben.post("/courses/"+id+"/chat/"+rid+"/delete", url.Values{}) // not Ben's: refused, nothing sent
	ada.post("/courses/"+id+"/chat/"+rid+"/delete", nil)
	if e := stream.next(); !strings.Contains(e, "Message deleted") || strings.Contains(e, "Hi Ben") {
		t.Fatalf("a Learner's stream does not show a deleted Message as deleted: %s", e)
	}
	adminStream.next()
	e = adminStream.next()
	expectInOrder(t, e, `id="m`+rid+`"`, "Message deleted")
	if strings.Contains(e, "Hi Ben") {
		t.Fatalf("a deleted Message's text is streamed: %s", e)
	}
}

func TestAChatEventSplitsAMessageOnEveryKindOfLineEnd(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	stream := ada.openStream(id)

	ben.say(id, "before\revent: x\rretry: 1\r\nafter")
	e := stream.next()
	if strings.Contains(e, "\r") {
		t.Fatalf("a bare carriage return reaches the stream, where it ends a line: %q", e)
	}
	expectInOrder(t, e, "before", "event: x", "retry: 1", "after")
}

func TestTheChatStreamIsRefusedToThoseWhoCannotReadTheChat(t *testing.T) {
	_, admin, _, id := chatSite(t)
	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")
	res, body := cara.get("/courses/" + id + "/chat/events")
	expectStatus(t, res, body, http.StatusForbidden, "")
	res, _ = admin.second().get("/courses/" + id + "/chat/events")
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a Visitor's stream got %d, want sent to log in", res.StatusCode)
	}
}

func TestTheChatsScriptGetsAMessagesFragmentInPlaceOfARedirect(t *testing.T) {
	ada, _, _, id := chatSite(t)
	req, err := http.NewRequest(http.MethodPost, ada.server.URL+"/courses/"+id+"/chat", strings.NewReader(url.Values{"body": {"From the script"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Chat", "1")
	res, body := ada.do(req)
	expectStatus(t, res, body, http.StatusOK, "From the script")
	if strings.Contains(body, "<html") {
		t.Fatalf("the script got a whole page: %s", body)
	}
	req, _ = http.NewRequest(http.MethodPost, ada.server.URL+"/courses/"+id+"/chat", strings.NewReader("body="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Chat", "1")
	res, body = ada.do(req)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Write a Message before sending.")
	if strings.Contains(body, "<html") {
		t.Fatalf("the script got a whole page for a refusal: %s", body)
	}

	_, body = ada.get("/courses/" + id + "/chat")
	expectInOrder(t, body, `data-stream="/courses/`+id+`/chat/events"`, `<script src="/static/chat.js`)
}

func TestDeletingAnAlreadyDeletedMessageSendsNothingToOpenStreams(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	stream := ada.openStream(id)
	mid := ben.say(id, "Oops")
	ada.reply(id, mid, "Hi Ben")
	stream.next()
	stream.next()
	ben.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	stream.next() // the Message, and the reply quoting it
	stream.next()

	res, body := ben.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasSuffix(loc, "#m"+mid) {
		t.Fatalf("a second delete redirected to %q, want the Chat at the Message", loc)
	}
	ben.say(id, "Later")
	if e := stream.next(); !strings.Contains(e, "Later") {
		t.Fatalf("a second delete sent the stream an event: %s", e)
	}
}
