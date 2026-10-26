package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	chatPageSize     = 50   // Messages a Chat page shows
	maxMessageLength = 1000 // characters
	// A Learner's posting budgets in a Chat; the Admin has none.
	messagesPerMinute  = 10
	reactionsPerMinute = 30
	reportsPerHour     = 10
	maxReportReason    = 500 // characters
)

// reactions are the six a Message can have, in the order they are shown: thumbs up, red heart, tears of joy, open
// mouth, folded hands and clapping hands. Written as escapes, as the code holds no emoji.
var reactions = []string{"\U0001F44D", "\u2764\uFE0F", "\U0001F602", "\U0001F62E", "\U0001F64F", "\U0001F44F"}

// chatReaction is one of the six Reactions on a Message: how many people put it there, and whether the viewer did.
type chatReaction struct {
	Emoji string
	Count int
	Mine  bool
}

// chatMessage is a Message as one viewer sees it, which is what the chat-message template reads. Text is empty when
// the Message is removed for the viewer, so removed text is never sent to them.
type chatMessage struct {
	ID, CourseID int64
	Writer       string
	ByAdmin      bool // the writer is the Admin
	Mine         bool // the viewer wrote it
	Text         string
	At           time.Time
	Deleted      bool // by its writer
	Hidden       bool // by the Admin
	// The Message this one replies to, 0 when none, with its writer and first line. A removed original, hidden,
	// deleted or gone with its writer's account, is quoted as removed, with neither.
	ReplyID      int64
	ReplyWriter  string
	ReplyLine    string
	ReplyRemoved bool
	Reactions    []chatReaction // all six, in order
	Reported     bool           // the viewer has Reported it
	ViewerAdmin  bool
	Lang         string
	writerID     int64 // who wrote it, which groups their Messages
	// Where the Message sits on its page: Follows when it comes within followsWithin of the one before it by the same
	// writer, so it shows under that one's name; Day is the date heading before it when it starts a day, else "".
	// A live Message is sent without either, and the script groups it.
	Follows bool
	Day     string
}

// Visible reports whether the Message's text is shown, so it may be replied to, reacted to and reported.
func (m chatMessage) Visible() bool { return !m.Deleted && !m.Hidden }

// Removed reports whether the viewer is shown "Message removed" in place of the text.
func (m chatMessage) Removed() bool { return m.Hidden && !m.ViewerAdmin && !m.Deleted }

// FirstLine is the first line of the Message's text, which a reply quotes.
func (m chatMessage) FirstLine() string { line, _, _ := strings.Cut(m.Text, "\n"); return line }

// Path is the address of the Message's Chat.
func (m chatMessage) Path() string { return chatPath(m.CourseID) }

// ReplyLink is the address of the Chat page that shows the Message this one replies to.
func (m chatMessage) ReplyLink() string {
	return fmt.Sprintf("%s?before=%d#m%d", m.Path(), m.ReplyID+1, m.ReplyID)
}

// communityChat is the Course ID that stands for the Community Chat, whose Messages have no Course.
const communityChat int64 = 0

// chatPath is the address of Course courseID's Chat, or of the Community Chat when courseID is 0.
func chatPath(courseID int64) string {
	if courseID == communityChat {
		return "/community"
	}
	return fmt.Sprintf("/courses/%d/chat", courseID)
}

// inChat is the SQL condition that Message m is in Course $3's Chat, or in the Community Chat when courseID, which
// is $3, is 0.
func inChat(courseID int64) string {
	if courseID == communityChat {
		return `m.course_id IS NULL AND $3::bigint = 0`
	}
	return `m.course_id = $3`
}

// When is the time the Message was posted.
func (m chatMessage) When() string { return dateTimeIn(m.Lang, m.At) }

// Clock is the time of day the Message was posted, as "11:14" Malaysia time.
func (m chatMessage) Clock() string { return m.At.In(malaysia).Format("15:04") }

// Initial is the first letter of the writer's name, which stands in for their picture.
func (m chatMessage) Initial() string {
	for _, r := range m.Writer {
		return strings.ToUpper(string(r))
	}
	return ""
}

// followsWithin is how soon after a writer's Message their next one shows under the same name.
const followsWithin = 5 * time.Minute

// groupByWriterAndDay sets where each Message of a page, oldest first, sits: under the previous one's name or not, and after a
// date heading or not.
func groupByWriterAndDay(list []chatMessage) {
	day := ""
	for i := range list {
		m := &list[i]
		if d := text.Date(m.Lang, m.At.In(malaysia)); d != day {
			m.Day, day = d, d
		} else if p := list[i-1]; p.writerID == m.writerID && m.At.Sub(p.At) < followsWithin {
			m.Follows = true
		}
	}
}

// chatPage is a Chat's page.
type chatPage struct {
	pageBase
	Course   course // the zero course for the Community Chat
	Messages []chatMessage
	Earlier  int64        // where "Show earlier" pages back to, 0 when there is nothing earlier
	Paged    bool         // an earlier page, not the latest Messages
	Body     string       // the text of a refused Message, kept for another try
	ReplyTo  *chatMessage // the Message the composer replies to, nil when none
}

// Path is the address of the page's Chat.
func (p chatPage) Path() string { return chatPath(p.Course.ID) }

// FollowsWithin is followsWithin in milliseconds, for the script that groups live Messages.
func (p chatPage) FollowsWithin() int64 { return followsWithin.Milliseconds() }

// chatViewer is who is reading a Chat.
type chatViewer struct {
	*learner
	lang string
}

// chatFor loads the Chat the request names, the Chat of the Course named by {id} or else the Community Chat, whose
// course is the zero one, and checks that the logged-in Learner may use it. When not, it writes the refusal and
// returns false.
func (a *App) chatFor(w http.ResponseWriter, r *http.Request) (course, chatViewer, bool) {
	var c course
	if r.PathValue("id") != "" {
		var ok bool
		if c, ok = a.courseFor(w, r); !ok {
			return c, chatViewer{}, false
		}
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return c, chatViewer{}, false
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return c, chatViewer{}, false
	}
	if c.ID != communityChat && !c.Published && !l.IsAdmin {
		a.courseNotFound(w, r)
		return c, chatViewer{}, false
	}
	allowed, err := a.mayChat(r.Context(), l, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return c, chatViewer{}, false
	}
	if !allowed {
		p := page{Learner: l, Course: &c, Message: text.T(lang, "chat.join-to-open")}
		if c.ID == communityChat {
			p.Course, p.Message = nil, text.T(lang, "community.join-to-open")
		}
		a.render(w, r, http.StatusForbidden, "members_only", p)
		return c, chatViewer{}, false
	}
	return c, chatViewer{l, lang}, true
}

// mayChat reports whether l may read and post in Course courseID's Chat, or the Community Chat when courseID is 0:
// the Admin, or a Learner enrolled in the Course or joined to the Community Chat.
func (a *App) mayChat(ctx context.Context, l *learner, courseID int64) (bool, error) {
	if l.IsAdmin {
		return true, nil
	}
	if courseID == communityChat {
		return a.communityMember(ctx, l.ID)
	}
	return a.enrolled(ctx, l.ID, courseID)
}

// chatColumns selects a Message as viewer $1, who is the Admin when $2, sees it. Removed text never leaves the database.
const chatColumns = `m.id, coalesce(m.course_id, 0), l.name, l.is_admin, m.learner_id = $1,
	CASE WHEN m.hidden AND NOT $2 THEN '' ELSE m.body END, m.created_at, m.deleted, m.hidden,
	coalesce(m.reply_to, 0), o.id IS NULL OR o.deleted OR o.hidden,
	CASE WHEN o.deleted OR o.hidden THEN '' ELSE coalesce(ol.name, '') END,
	CASE WHEN o.deleted OR o.hidden THEN '' ELSE coalesce(split_part(o.body, E'\n', 1), '') END,
	EXISTS (SELECT 1 FROM chat_reports rp WHERE rp.message_id = m.id AND rp.learner_id = $1), m.learner_id
	FROM chat_messages m JOIN learners l ON l.id = m.learner_id
	LEFT JOIN chat_messages o ON o.id = m.reply_to LEFT JOIN learners ol ON ol.id = o.learner_id`

// chatRefusal refuses an action in a Chat: with the text under key to the Chat's script, else on a page titled
// under title.
func (a *App) chatRefusal(w http.ResponseWriter, r *http.Request, v chatViewer, status int, title, key string) {
	if !refuseInPlace(w, r, status, text.T(v.lang, key)) {
		a.render(w, r, status, "message", page{Title: text.T(v.lang, title), Message: text.T(v.lang, key)})
	}
}

// chatBudget records one of v's actions of kind in Course courseID's Chat, or the Community Chat, and reports whether
// it is within limit in window. Each Chat has its own count. The Admin has no limit.
func (a *App) chatBudget(v chatViewer, courseID int64, kind string, limit int, window time.Duration) bool {
	return v.IsAdmin || a.limiter.allow(chatBudgetKey(v, courseID, kind), limit, window, a.now())
}

// chatBudgetKey is the limiter key of v's actions of kind in Course courseID's Chat, or the Community Chat.
func chatBudgetKey(v chatViewer, courseID int64, kind string) string {
	return fmt.Sprintf("chat %s %d %d", kind, courseID, v.ID)
}

// chatMessages loads the Messages that where picks, as v sees them. where's own arguments start at $3.
func (a *App) chatMessages(ctx context.Context, v chatViewer, where string, args ...any) ([]chatMessage, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+chatColumns+` WHERE `+where, append([]any{v.ID, v.IsAdmin}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []chatMessage
	var ids []int64
	at := map[int64]int{} // index in list by Message ID
	for rows.Next() {
		m := chatMessage{ViewerAdmin: v.IsAdmin, Lang: v.lang, Reactions: make([]chatReaction, len(reactions))}
		for i, e := range reactions {
			m.Reactions[i].Emoji = e
		}
		if err := rows.Scan(&m.ID, &m.CourseID, &m.Writer, &m.ByAdmin, &m.Mine, &m.Text, &m.At, &m.Deleted, &m.Hidden,
			&m.ReplyID, &m.ReplyRemoved, &m.ReplyWriter, &m.ReplyLine, &m.Reported, &m.writerID); err != nil {
			return nil, err
		}
		at[m.ID] = len(list)
		ids = append(ids, m.ID)
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, a.countReactions(ctx, v, ids, func(id int64, i int, r chatReaction) { list[at[id]].Reactions[i] = r })
}

// countReactions counts the Reactions on the Messages ids, calling put with each Message's i-th Reaction found.
func (a *App) countReactions(ctx context.Context, v chatViewer, ids []int64, put func(id int64, i int, r chatReaction)) error {
	rows, err := a.db.QueryContext(ctx, `SELECT message_id, emoji, count(*), bool_or(learner_id = $2) FROM chat_reactions
		WHERE message_id = ANY($1) GROUP BY message_id, emoji`, ids, v.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var r chatReaction
		if err := rows.Scan(&id, &r.Emoji, &r.Count, &r.Mine); err != nil {
			return err
		}
		put(id, slices.Index(reactions, r.Emoji), r)
	}
	return rows.Err()
}

// chat is a Course's Chat page: the latest Messages, oldest first, or with ?before= the ones before that Message.
func (a *App) chat(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	var p chatPage
	if to := r.URL.Query().Get("reply"); to != "" {
		if p.ReplyTo, ok = a.replyTo(w, r, c, v, to); !ok {
			return
		}
	}
	a.renderChat(w, r, http.StatusOK, c, v, p)
}

// replyTo loads the Message with ID to in Course c to reply to, as v sees it, or nil when it is not there to reply to:
// in another Course, deleted or hidden. It returns false when it has written an error page.
func (a *App) replyTo(w http.ResponseWriter, r *http.Request, c course, v chatViewer, to string) (*chatMessage, bool) {
	id, err := strconv.ParseInt(to, 10, 64)
	if err != nil {
		return nil, true
	}
	list, err := a.chatMessages(r.Context(), v, inChat(c.ID)+` AND m.id = $4 AND NOT m.deleted AND NOT m.hidden`, c.ID, id)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	if len(list) == 0 {
		return nil, true
	}
	return &list[0], true
}

// renderChat renders the Chat page p of Course c, or the Community Chat, for v, loading its Messages.
func (a *App) renderChat(w http.ResponseWriter, r *http.Request, status int, c course, v chatViewer, p chatPage) {
	before := int64(math.MaxInt64)
	if b, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64); err == nil {
		before, p.Paged = b, true
	}
	list, err := a.chatMessages(r.Context(), v, inChat(c.ID)+` AND m.id < $4 ORDER BY m.id DESC LIMIT $5`, c.ID, before, chatPageSize+1)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(list) > chatPageSize {
		list = list[:chatPageSize]
		p.Earlier = list[chatPageSize-1].ID
	}
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	groupByWriterAndDay(list)
	p.Course, p.Messages, p.Learner = c, list, v.learner
	a.render(w, r, status, "chat", &p)
}

// postMessage is a Learner or the Admin posting a Message to a Course's Chat.
func (a *App) postMessage(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	body := strings.ReplaceAll(r.PostFormValue("body"), "\r\n", "\n")
	var replyTo *chatMessage
	if to := r.PostFormValue("reply_to"); to != "" {
		if replyTo, ok = a.replyTo(w, r, c, v, to); !ok {
			return
		}
	}
	refuse := func(status int, key string) {
		if !refuseInPlace(w, r, status, text.T(v.lang, key)) {
			a.renderChat(w, r, status, c, v, chatPage{pageBase: pageBase{Error: text.T(v.lang, key)}, Body: body, ReplyTo: replyTo})
		}
	}
	switch {
	case r.PostFormValue("reply_to") != "" && replyTo == nil:
		refuse(http.StatusUnprocessableEntity, "chat.err.reply")
		return
	case strings.TrimSpace(body) == "":
		refuse(http.StatusUnprocessableEntity, "chat.err.empty")
		return
	case utf8.RuneCountInString(body) > maxMessageLength:
		refuse(http.StatusUnprocessableEntity, "chat.err.too-long")
		return
	case !a.chatBudget(v, c.ID, "message", messagesPerMinute, time.Minute):
		refuse(http.StatusTooManyRequests, "chat.err.too-fast")
		return
	}
	var id int64
	var replyID *int64
	if replyTo != nil {
		replyID = &replyTo.ID
	}
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO chat_messages (course_id, learner_id, body, reply_to)
		VALUES (nullif($1::bigint, 0), $2, $3, $4) RETURNING id`, c.ID, v.ID, body, replyID).Scan(&id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	m := chatMessage{ID: id, CourseID: c.ID}
	a.changed(r.Context(), m, false)
	a.answer(w, r, v, m)
}

// chatMessageFor loads the Message named by {mid} in Course c as v sees it. When there is none it writes the Not
// found page and returns false.
func (a *App) chatMessageFor(w http.ResponseWriter, r *http.Request, c course, v chatViewer) (chatMessage, bool) {
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return chatMessage{}, false
	}
	list, err := a.chatMessages(r.Context(), v, inChat(c.ID)+` AND m.id = $4`, c.ID, mid)
	if err != nil {
		a.serverError(w, r, err)
		return chatMessage{}, false
	}
	if len(list) == 0 {
		a.notFound(w, r)
		return chatMessage{}, false
	}
	return list[0], true
}

// deletableMessage loads the Message named by the path for its writer. For anyone else it writes the refusal and
// returns false.
func (a *App) deletableMessage(w http.ResponseWriter, r *http.Request) (course, chatViewer, chatMessage, bool) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return c, v, chatMessage{}, false
	}
	m, ok := a.chatMessageFor(w, r, c, v)
	if !ok {
		return c, v, m, false
	}
	if !m.Mine {
		if !refuseInPlace(w, r, http.StatusForbidden, text.T(v.lang, "chat.not-yours")) {
			a.render(w, r, http.StatusForbidden, "message", page{Title: text.T(v.lang, "chat.not-yours.title"), Message: text.T(v.lang, "chat.not-yours")})
		}
		return c, v, m, false
	}
	return c, v, m, true
}

// confirmDeleteMessage asks the writer to confirm deleting their Message.
func (a *App) confirmDeleteMessage(w http.ResponseWriter, r *http.Request) {
	_, _, m, ok := a.deletableMessage(w, r)
	if !ok {
		return
	}
	back, err := a.messageLink(r.Context(), m)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.confirm(w, r, http.StatusOK, confirmation{Heading: "chat.delete-title", Question: "chat.delete-ask", Cancel: back, Action: r.URL.Path})
}

// deleteMessage is a writer deleting their own Message, once its confirmation page is posted: its text is emptied
// and it shows "Message deleted".
func (a *App) deleteMessage(w http.ResponseWriter, r *http.Request) {
	c, v, m, ok := a.deletableMessage(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if !a.chatBudget(v, c.ID, "delete", messagesPerMinute, time.Minute) {
		a.chatRefusal(w, r, v, http.StatusTooManyRequests, "limited.title", "chat.err.too-fast")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE chat_messages SET deleted = true, body = '' WHERE id = $1 AND NOT deleted`, m.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 { // deleted already: nothing changed, so nothing is republished
		a.answer(w, r, v, m)
		return
	}
	if err := a.closeReports(r.Context(), m.ID); err != nil { // nothing left to act on
		a.serverError(w, r, err)
		return
	}
	a.changed(r.Context(), m, true)
	a.answer(w, r, v, m)
}

// react is a Learner or the Admin putting a Reaction on a Message, changing theirs to it, or taking it off when it
// is theirs already.
func (a *App) react(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	m, ok := a.chatMessageFor(w, r, c, v)
	if !ok {
		return
	}
	refuse := func(status int, title, key string) { a.chatRefusal(w, r, v, status, title, key) }
	emoji := r.PostFormValue("emoji")
	switch {
	case !slices.Contains(reactions, emoji) || !m.Visible():
		refuse(http.StatusUnprocessableEntity, "chat.no-reaction.title", "chat.no-reaction")
		return
	case !a.chatBudget(v, c.ID, "reaction", reactionsPerMinute, time.Minute):
		refuse(http.StatusTooManyRequests, "limited.title", "chat.err.too-fast")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `WITH off AS (
			DELETE FROM chat_reactions WHERE message_id = $1 AND learner_id = $2 AND emoji = $3 RETURNING 1)
		INSERT INTO chat_reactions (message_id, learner_id, emoji) SELECT $1, $2, $3 WHERE NOT EXISTS (SELECT 1 FROM off)
		ON CONFLICT (message_id, learner_id) DO UPDATE SET emoji = EXCLUDED.emoji`, m.ID, v.ID, emoji)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.changed(r.Context(), m, false)
	a.answer(w, r, v, m)
}

// reportMessage is a Learner Reporting a Message to the Admin, with an optional reason. The Message stays visible.
func (a *App) reportMessage(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	m, ok := a.chatMessageFor(w, r, c, v)
	if !ok {
		return
	}
	refuse := func(status int, title, key string) { a.chatRefusal(w, r, v, status, title, key) }
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	switch {
	case v.IsAdmin || m.Mine: // the Admin hides; a writer deletes
		refuse(http.StatusUnprocessableEntity, "chat.not-reported", "chat.err.report-own")
		return
	case !m.Visible():
		refuse(http.StatusUnprocessableEntity, "chat.not-reported", "chat.err.report-removed")
		return
	case utf8.RuneCountInString(reason) > maxReportReason:
		refuse(http.StatusUnprocessableEntity, "chat.not-reported", "chat.err.reason-too-long")
		return
	case !a.chatBudget(v, c.ID, "report", reportsPerHour, time.Hour):
		refuse(http.StatusTooManyRequests, "limited.title", "chat.err.reports-limited")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO chat_reports (message_id, learner_id, reason) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, m.ID, v.ID, reason)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.limiter.forget(chatBudgetKey(v, c.ID, "report")) // a Report refused as a repeat doesn't count
		refuse(http.StatusConflict, "chat.not-reported", "chat.err.reported")
		return
	}
	a.answer(w, r, v, m)
}

// confirmHide asks the Admin to confirm hiding a Message. Cancel returns to the Reports page when back is "reports",
// else to the Chat at the Message.
func (a *App) confirmHide(w http.ResponseWriter, r *http.Request) {
	var m chatMessage
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT id, coalesce(course_id, 0) FROM chat_messages WHERE id = $1`, mid).Scan(&m.ID, &m.CourseID)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c := confirmation{Side: "reports", Heading: "chat.hide-title", Question: "chat.hide-ask", Cancel: "/admin/reports", Action: r.URL.RequestURI()}
	if r.FormValue("back") != "reports" {
		c.Side = ""
		if c.Cancel, err = a.messageLink(r.Context(), m); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.confirm(w, r, http.StatusOK, c)
}

// setHidden returns the handler with which the Admin hides or unhides a Message; hiding waits for its confirmation
// page. Hiding closes every open Report on it. back "reports" returns to the Reports page, else to the Chat at the
// Message.
func (a *App) setHidden(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hidden && unconfirmed(w, r) {
			return
		}
		mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
		if err != nil {
			a.notFound(w, r)
			return
		}
		var m chatMessage
		err = a.db.QueryRowContext(r.Context(), `UPDATE chat_messages SET hidden = $2 WHERE id = $1 RETURNING id, coalesce(course_id, 0)`,
			mid, hidden).Scan(&m.ID, &m.CourseID)
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w, r)
			return
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if hidden {
			if err := a.closeReports(r.Context(), m.ID); err != nil {
				a.serverError(w, r, err)
				return
			}
		}
		a.changed(r.Context(), m, true)
		if r.FormValue("back") == "reports" {
			http.Redirect(w, r, "/admin/reports", http.StatusSeeOther)
			return
		}
		l, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.answer(w, r, chatViewer{l, lang}, m)
	}
}

// closeReports closes every open Report on Message id.
func (a *App) closeReports(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx, `UPDATE chat_reports SET closed_at = now() WHERE message_id = $1 AND closed_at IS NULL`, id)
	return err
}

// backToMessage redirects to the Chat page that shows m: the latest, or an earlier one ending with it.
func (a *App) backToMessage(w http.ResponseWriter, r *http.Request, m chatMessage) {
	to, err := a.messageLink(r.Context(), m)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// messageLink is the address of Message m in its Chat, on the earlier page that shows it when it is not on the latest.
func (a *App) messageLink(ctx context.Context, m chatMessage) (string, error) {
	var newer int
	err := a.db.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT 1 FROM chat_messages m WHERE `+inChat(m.CourseID)+`
		AND m.id > $1 LIMIT $2) n`, m.ID, chatPageSize, m.CourseID).Scan(&newer)
	if err != nil {
		return "", err
	}
	if newer >= chatPageSize {
		return fmt.Sprintf("%s?before=%d#m%d", m.Path(), m.ID+1, m.ID), nil
	}
	return fmt.Sprintf("%s#m%d", m.Path(), m.ID), nil
}
