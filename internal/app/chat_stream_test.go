package app_test

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// chatStream is an open event stream of a Course's Chat; events carries each event's data.
type chatStream struct {
	t      *testing.T
	events chan string
	done   chan struct{} // closed when the server ends the stream
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
	st := &chatStream{t: s.t, events: make(chan string, 100), done: make(chan struct{})}
	go func() {
		defer close(st.done)
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

// ends waits for the server to end the stream, which must carry no further event.
func (st *chatStream) ends() {
	st.t.Helper()
	select {
	case e := <-st.events:
		st.t.Fatalf("the Chat stream carried an event when it should have ended: %s", e)
	case <-st.done:
		select {
		case e := <-st.events:
			st.t.Fatalf("the Chat stream carried an event before it ended: %s", e)
		default:
		}
	case <-time.After(5 * time.Second):
		st.t.Fatal("the Chat stream did not end within 5 seconds")
	}
}

func TestAChatStreamEndsWhenItsLearnerLogsOut(t *testing.T) {
	t.Parallel()
	ada, _, ben, id := chatSite(t)
	stream := ada.openStream(id)
	ada.post("/logout", nil)
	ben.say(id, "After the logout")
	stream.ends()
}

func TestAChatStreamEndsWhenAPasswordResetSignsItsLearnerOut(t *testing.T) {
	t.Parallel()
	ada, admin, ben, id := chatSite(t)
	stream := ada.openStream(id)
	owner := admin.second()
	owner.forgotPassword("ada@example.com")
	owner.resetPassword(resetToken(t, owner.lastEmailLinkPath("ada@example.com", "/reset-password")), "a brand new passphrase")
	ben.say(id, "After the reset")
	stream.ends()
}

func TestAnOpenChatStreamReceivesNewAndChangedMessages(t *testing.T) {
	t.Parallel()
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

	admin.post("/admin/chat/"+mid+"/hide", confirming)
	for _, e := range []string{stream.next(), stream.next()} { // the Message, and the reply quoting it
		if strings.Contains(e, "Live hello") {
			t.Fatalf("a Learner's stream carries a hidden Message's text: %s", e)
		}
	}
	e = adminStream.next()
	expectInOrder(t, e, `is-hidden" id="m`+mid+`"`, "Live hello")

	ben.post("/courses/"+id+"/chat/"+rid+"/delete", url.Values{}) // not Ben's: refused, nothing sent
	ada.post("/courses/"+id+"/chat/"+rid+"/delete", confirming)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ada, _, ben, id := chatSite(t)
	stream := ada.openStream(id)
	mid := ben.say(id, "Oops")
	ada.reply(id, mid, "Hi Ben")
	stream.next() // the Message, then the reply
	stream.next()
	ben.post("/courses/"+id+"/chat/"+mid+"/delete", confirming)
	stream.next() // the Message, and the reply quoting it
	stream.next()

	res, body := ben.post("/courses/"+id+"/chat/"+mid+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasSuffix(loc, "#m"+mid) {
		t.Fatalf("a second delete redirected to %q, want the Chat at the Message", loc)
	}
	ben.say(id, "Later")
	if e := stream.next(); !strings.Contains(e, "Later") {
		t.Fatalf("a second delete sent the stream an event: %s", e)
	}
}

// sessionToken is the value of s's session cookie.
func (s *site) sessionToken() string {
	s.t.Helper()
	u, err := url.Parse(s.server.URL)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, c := range s.client.Jar.Cookies(u) {
		if c.Name == app.SessionCookie {
			return c.Value
		}
	}
	s.t.Fatal("no session cookie")
	return ""
}

// expectSharedFragments checks that every one of mids renders for each of viewers from a change loaded once and
// shared by all their streams exactly as from the viewer's own queries.
func expectSharedFragments(t *testing.T, ada *site, id string, mids []string, viewers map[string]*site) {
	t.Helper()
	courseID, _ := strconv.ParseInt(id, 10, 64)
	var names, tokens []string
	for name, s := range viewers {
		names, tokens = append(names, name), append(tokens, s.sessionToken())
	}
	for _, mid := range mids {
		messageID, _ := strconv.ParseInt(mid, 10, 64)
		shared, apart, err := ada.application.FragmentsBothWays(courseID, messageID, tokens)
		if err != nil {
			t.Fatal(err)
		}
		for i, name := range names {
			if shared[i] == "" || shared[i] != apart[i] {
				t.Errorf("Message %s as %s: a shared change renders\n%s\nbut its own queries render\n%s", mid, name, shared[i], apart[i])
			}
		}
	}
}

func TestASharedChangeRendersEachViewersFragmentAsTheirOwnQueriesDo(t *testing.T) {
	t.Parallel()
	ada, admin, ben, id := chatSite(t)
	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")
	expectStatus(t, must(cara.post("/courses/"+id+"/join", nil)), "", http.StatusSeeOther, "")
	viewers := map[string]*site{"writer": ada, "reporter": ben, "reactor": cara, "admin": admin}

	// A reply with Reactions from Cara and Ada and a Report from Ben, quoting a Message that has a Reaction of its own.
	mid := ben.say(id, "Original\nsecond line")
	cara.react(id, mid, clap)
	rid := ada.reply(id, mid, "Hi Ben")
	cara.react(id, rid, thumbsUp)
	ada.react(id, rid, laugh)
	expectStatus(t, must(ben.report(id, rid, "Spam")), "", http.StatusSeeOther, "")
	expectSharedFragments(t, ada, id, []string{mid, rid}, viewers)

	admin.post("/admin/chat/"+mid+"/hide", confirming) // the original is hidden, so the reply quotes it as removed
	expectSharedFragments(t, ada, id, []string{mid, rid}, viewers)

	ada.post("/courses/"+id+"/chat/"+rid+"/delete", confirming)
	expectSharedFragments(t, ada, id, []string{mid, rid}, viewers)
}

func TestSeveralOpenStreamsEachGetTheirOwnViewOfOneMessage(t *testing.T) {
	t.Parallel()
	ada, admin, ben, id := chatSite(t)
	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")
	expectStatus(t, must(cara.post("/courses/"+id+"/join", nil)), "", http.StatusSeeOther, "")
	streams := map[*site]*chatStream{ada: ada.openStream(id), ben: ben.openStream(id), cara: cara.openStream(id), admin: admin.openStream(id)}

	mid := ben.say(id, "For everyone")
	for s, st := range streams {
		e := st.next()
		expectInOrder(t, e, `id="m`+mid+`"`, "For everyone")
		if mine := strings.Contains(e, "is-mine"); mine != (s == ben) {
			t.Errorf("a stream is told the Message is its own: %t, want %t: %s", mine, s == ben, e)
		}
	}
	ben.post("/courses/"+id+"/chat/"+mid+"/delete", confirming)
	for _, st := range streams {
		if e := st.next(); !strings.Contains(e, "Message deleted") {
			t.Errorf("a stream does not see the deletion: %s", e)
		}
	}
	// A stream whose session ended stops, the others carry on.
	cara.post("/logout", nil)
	ada.say(id, "After Cara left")
	streams[cara].ends()
	for _, s := range []*site{ada, ben, admin} {
		expectInOrder(t, streams[s].next(), "After Cara left")
	}
}

func TestAChatStreamEndsWhenItsLearnerLeavesTheCourse(t *testing.T) {
	t.Parallel()
	ada, _, ben, id := chatSite(t)
	gone, staying := ada.openStream(id), ben.openStream(id)
	expectStatus(t, must(ada.leaveCourse(id)), "", http.StatusSeeOther, "")
	ben.say(id, "After Ada left")
	gone.ends()
	expectInOrder(t, staying.next(), "After Ada left")
}
