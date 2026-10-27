package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
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
	streams map[int64]map[chan *chatChange][]byte // by Course, 0 for the Community Chat; each with its session token's hash
}

// chatChange is Message id of Course courseID being new or changed. Every stream of the Chat gets the same one, and
// the first to need it loads it for all, so a change costs a fixed handful of queries however many streams are open.
type chatChange struct {
	courseID, id int64
	tokens       [][]byte // the session token hashes of the streams it was published to
	once         sync.Once
	loaded       *loadedChange
	err          error
}

// loadedChange is what a chatChange holds once loaded: all that does not depend on who is looking.
type loadedChange struct {
	m         chatMessage       // with its text as the Admin sees it, and nothing set for a viewer; zero when gone
	found     bool              // the Message is still there
	reacted   [][]int64         // who put each of the reactions on it, in order
	reporters []int64           // who Reported it
	viewers   map[string]access // by session token hash

	mu       sync.Mutex
	rendered map[string]string // the fragment by the parts of seenBy that differ between viewers
}

// access is a Learner a session token still belongs to, and whether they may still read the Chat.
type access struct {
	l       learner
	allowed bool
}

// subscribe opens a stream of Course courseID's new and changed Messages, for the session with token hash token.
func (h *chatHub) subscribe(courseID int64, token []byte) chan *chatChange {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams == nil {
		h.streams = map[int64]map[chan *chatChange][]byte{}
	}
	if h.streams[courseID] == nil {
		h.streams[courseID] = map[chan *chatChange][]byte{}
	}
	ch := make(chan *chatChange, 64)
	h.streams[courseID][ch] = token
	return ch
}

// unsubscribe closes a stream subscribe opened.
func (h *chatHub) unsubscribe(courseID int64, ch chan *chatChange) {
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
	if len(h.streams[courseID]) == 0 {
		return
	}
	change := &chatChange{courseID: courseID, id: id}
	for _, token := range h.streams[courseID] {
		change.tokens = append(change.tokens, token)
	}
	for ch := range h.streams[courseID] {
		select {
		case ch <- change:
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
	var token []byte // a request without the session cookie got no further than chatFor
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		token = hashToken(cookie.Value)
	}
	changes := a.chatHub.subscribe(c.ID, token)
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
	// currentLearner, which keeps the first lookup for the whole request, and renders from then on as the fresh Learner.
	// A change does this once for all streams, in streamFragment; a ping, once per stream.
	// ponytail: two queries per ping per stream; fine for a Course's Learners, cache it if Chats grow large.
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
		case change := <-changes:
			html, ok, err := a.streamFragment(change, &v, token)
			if err != nil {
				a.logError(r, err)
				return
			}
			if !ok {
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

// streamFragment renders the Message that change is of for the stream of session token hash token, whose viewer v it
// refreshes, as messageFragment would after stillAllowed, but with no query of its own: the change loads once for all
// its streams. It reports false when the stream must end, because the session or the right to the Chat is gone, and
// returns "" when the Message is gone.
func (a *App) streamFragment(change *chatChange, v *chatViewer, token []byte) (string, bool, error) {
	change.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // not a stream's own: one ending must not fail the rest
		defer cancel()
		change.loaded, change.err = a.loadChange(ctx, change)
	})
	if change.err != nil {
		return "", false, change.err
	}
	s := change.loaded
	who, ok := s.viewers[string(token)]
	if !ok || !who.allowed || who.l.ID != v.ID {
		return "", false, nil
	}
	v.learner = &who.l
	if !s.found {
		return "", true, nil
	}
	m := s.seenBy(*v)
	// Most of a Chat sees a Message alike, so each way of seeing it is rendered once: rendering, not the queries, is
	// what a whole class's streams spend their time on. The key is all of m, so whatever seenBy comes to set keeps
	// one viewer's fragment from reaching another.
	key := fmt.Sprintf("%#v", m)
	s.mu.Lock()
	html, done := s.rendered[key]
	s.mu.Unlock()
	if done {
		return html, true, nil
	}
	var buf bytes.Buffer
	if err := a.pages["chat"].ExecuteTemplate(&buf, "chat-message", m); err != nil {
		return "", false, err
	}
	html = strings.TrimSpace(buf.String())
	s.mu.Lock()
	s.rendered[key] = html
	s.mu.Unlock()
	return html, true, nil
}

// loadChange loads change in four queries: the Message, who reacted to it and how, who Reported it, and whether each of
// its streams' sessions is still good for the Chat, which is what mayChat and lookUpLearner ask of one stream.
func (a *App) loadChange(ctx context.Context, change *chatChange) (*loadedChange, error) {
	s := &loadedChange{reacted: make([][]int64, len(reactions)), viewers: map[string]access{}, rendered: map[string]string{}}
	// As the Admin who is nobody sees it: the text whole, and for nobody Mine, Reported or Reacted.
	rows, err := a.db.QueryContext(ctx, `SELECT `+chatColumns+` WHERE `+inChat(change.courseID)+` AND m.id = $4`,
		0, true, change.courseID, change.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		if s.m, err = scanChatMessage(rows, chatViewer{&learner{IsAdmin: true}, ""}); err != nil {
			return nil, err
		}
		s.found = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if s.found {
		if err := a.loadWhoActed(ctx, change.id, s); err != nil {
			return nil, err
		}
	}
	return s, a.loadViewers(ctx, change, s)
}

// loadWhoActed sets who put each Reaction on Message id, and who Reported it.
func (a *App) loadWhoActed(ctx context.Context, id int64, s *loadedChange) error {
	rows, err := a.db.QueryContext(ctx, `SELECT emoji, learner_id FROM chat_reactions WHERE message_id = $1`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var emoji string
		var who int64
		if err := rows.Scan(&emoji, &who); err != nil {
			return err
		}
		if i := slices.Index(reactions, emoji); i >= 0 {
			s.reacted[i] = append(s.reacted[i], who)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	rows, err = a.db.QueryContext(ctx, `SELECT learner_id FROM chat_reports WHERE message_id = $1`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var who int64
		if err := rows.Scan(&who); err != nil {
			return err
		}
		s.reporters = append(s.reporters, who)
	}
	return rows.Err()
}

// loadViewers sets s.viewers, the Learner behind each of change's session tokens that still has a session, with
// whether they may still read the Chat: the Admin, or enrolled in the Course or joined to the Community Chat.
func (a *App) loadViewers(ctx context.Context, change *chatChange, s *loadedChange) error {
	member, args := `EXISTS (SELECT 1 FROM community_members WHERE learner_id = l.id)`, []any{change.tokens, a.ownerEmail}
	if change.courseID != communityChat {
		member, args = `EXISTS (SELECT 1 FROM enrollments WHERE learner_id = l.id AND course_id = $3)`, append(args, change.courseID)
	}
	rows, err := a.db.QueryContext(ctx, `SELECT s.token_hash, `+learnerColumns+`, l.is_admin OR `+member+`
		FROM sessions s JOIN learners l ON l.id = s.learner_id
		WHERE s.token_hash = ANY($1) AND s.expires_at > now()`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var token []byte
		var who access
		if err := rows.Scan(&token, &who.l.ID, &who.l.Name, &who.l.IsAdmin, &who.l.IsOwner, &who.l.Lang, &who.l.PictureKey, &who.l.Username, &who.allowed); err != nil {
			return err
		}
		s.viewers[string(token)] = who
	}
	return rows.Err()
}

// seenBy is the loaded Message as v sees it, the same as chatColumns and countReactions read it for v.
func (s *loadedChange) seenBy(v chatViewer) chatMessage {
	m := s.m
	m.Mine, m.ViewerAdmin, m.Lang = m.writerID == v.ID, v.IsAdmin, v.lang
	m.Reported = slices.Contains(s.reporters, v.ID)
	if m.Hidden && !v.IsAdmin {
		m.Text = ""
	}
	m.Reactions = make([]chatReaction, len(reactions))
	for i, emoji := range reactions {
		m.Reactions[i] = chatReaction{emoji, len(s.reacted[i]), slices.Contains(s.reacted[i], v.ID)}
	}
	return m
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
