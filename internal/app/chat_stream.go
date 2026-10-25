package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// chatPing is how often an idle Chat stream sends a comment, so proxies keep it open.
const chatPing = 25 * time.Second

// chatHub tells each open Chat stream which of its Course's Messages are new or changed.
// ponytail: in-process, right for one server; a second server would need Postgres LISTEN/NOTIFY.
type chatHub struct {
	mu      sync.Mutex
	streams map[int64]map[chan int64]bool // by Course, 0 for the Community Chat
}

// subscribe opens a stream of the IDs of Course courseID's new and changed Messages.
func (h *chatHub) subscribe(courseID int64) chan int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams == nil {
		h.streams = map[int64]map[chan int64]bool{}
	}
	if h.streams[courseID] == nil {
		h.streams[courseID] = map[chan int64]bool{}
	}
	ch := make(chan int64, 64)
	h.streams[courseID][ch] = true
	return ch
}

// unsubscribe closes a stream subscribe opened.
func (h *chatHub) unsubscribe(courseID int64, ch chan int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.streams[courseID], ch)
	if len(h.streams[courseID]) == 0 {
		delete(h.streams, courseID)
	}
}

// publish tells every stream of Course courseID that Message id is new or changed.
// ponytail: a stream 64 changes behind skips the rest; the browser shows them on its next reload.
func (h *chatHub) publish(courseID, id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.streams[courseID] {
		select {
		case ch <- id:
		default:
		}
	}
}

// changed publishes Message m and, when its text was removed or restored, the replies quoting it.
func (a *App) changed(ctx context.Context, m chatMessage, withReplies bool) {
	a.chatHub.publish(m.CourseID, m.ID)
	if !withReplies {
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM chat_messages WHERE reply_to = $1 ORDER BY id`, m.ID)
	if err != nil {
		return // the replies show the change on the next reload
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			a.chatHub.publish(m.CourseID, id)
		}
	}
}

// sseLines starts a new data field at every kind of line end (CRLF, CR, LF), which an event stream parser reads as one.
var sseLines = strings.NewReplacer("\r\n", "\ndata: ", "\r", "\ndata: ", "\n", "\ndata: ")

// chatEvents is a Chat's Server-Sent Events stream: each new or changed Message, rendered as its viewer sees it.
// The browser reconnects by itself when the stream drops, after a restart or deploy.
func (a *App) chatEvents(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	changes := a.chatHub.subscribe(c.ID)
	defer a.chatHub.unsubscribe(c.ID, changes)
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 5000\n\n")
	if rc.Flush() != nil {
		return
	}
	// stillAllowed reports whether the stream may go on, as access may have ended since it opened: signed out,
	// session revoked or expired, left or removed. It looks the session up again with lookUpLearner, not
	// currentLearner, which keeps the first lookup for the whole request.
	// ponytail: two queries per change and per ping per stream; fine for a class, cache it if Chats grow large.
	stillAllowed := func() bool {
		l, err := a.lookUpLearner(r)
		if err != nil || l == nil || l.ID != v.ID {
			return false
		}
		if allowed, err := a.mayChat(r.Context(), l, c.ID); err != nil || !allowed {
			return false
		}
		v.learner = l
		return true
	}
	ping := time.NewTicker(chatPing)
	defer ping.Stop()
	for {
		var event string
		select {
		case <-r.Context().Done():
			return
		case <-a.done:
			return
		case <-ping.C:
			if !stillAllowed() {
				return
			}
			event = ": ping\n\n"
		case id := <-changes:
			if !stillAllowed() {
				return
			}
			html, err := a.messageFragment(r.Context(), v, c.ID, id)
			if err != nil {
				a.logError(r, err)
				return
			}
			if html == "" {
				continue
			}
			event = "data: " + sseLines.Replace(html) + "\n\n"
		}
		if _, err := fmt.Fprint(w, event); err != nil || rc.Flush() != nil {
			return
		}
	}
}

// messageFragment renders Message id of Course courseID, or the Community Chat, as v sees it, or "" when there is no such Message.
func (a *App) messageFragment(ctx context.Context, v chatViewer, courseID, id int64) (string, error) {
	list, err := a.chatMessages(ctx, v, inChat(courseID)+` AND m.id = $4`, courseID, id)
	if err != nil || len(list) == 0 {
		return "", err
	}
	var buf bytes.Buffer
	if err := a.pages["chat"].ExecuteTemplate(&buf, "chat-message", list[0]); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

// inPlace reports whether r comes from the Chat's script, which takes a Message's fragment or a refusal's text in
// place of a redirect or a page.
func inPlace(r *http.Request) bool { return r.Header.Get("X-Chat") == "1" }

// answer finishes an action on Message m: its fragment to the Chat's script, else a redirect to it in the Chat.
func (a *App) answer(w http.ResponseWriter, r *http.Request, v chatViewer, m chatMessage) {
	if !inPlace(r) {
		a.backToMessage(w, r, m)
		return
	}
	html, err := a.messageFragment(r.Context(), v, m.CourseID, m.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, html)
}

// refuseInPlace answers the Chat's script with msg as plain text and reports true. For a plain form it reports false,
// leaving the caller to render a page.
func refuseInPlace(w http.ResponseWriter, r *http.Request, status int, msg string) bool {
	if !inPlace(r) {
		return false
	}
	http.Error(w, msg, status)
	return true
}
