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

// reactions are the six a Message can have, in the order they are shown.
var reactions = []string{"👍", "❤️", "😂", "😮", "🙏", "👏"}

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
}

// Visible reports whether the Message's text is shown, so it may be replied to, reacted to and reported.
func (m chatMessage) Visible() bool { return !m.Deleted && !m.Hidden }

// Removed reports whether the viewer is shown "Message removed" in place of the text.
func (m chatMessage) Removed() bool { return m.Hidden && !m.ViewerAdmin && !m.Deleted }

// FirstLine is the first line of the Message's text, which a reply quotes.
func (m chatMessage) FirstLine() string { line, _, _ := strings.Cut(m.Text, "\n"); return line }

// ReplyLink is the address of the Chat page that shows the Message this one replies to.
func (m chatMessage) ReplyLink() string {
	return fmt.Sprintf("/courses/%d/chat?before=%d#m%d", m.CourseID, m.ReplyID+1, m.ReplyID)
}

// When is the time the Message was posted, Malaysia time.
func (m chatMessage) When() string {
	at := m.At.In(malaysia)
	return text.Date(m.Lang, at) + ", " + at.Format("15:04")
}

// chatPage is a Course's Chat page.
type chatPage struct {
	pageBase
	Course   course
	Messages []chatMessage
	Earlier  int64        // where "Show earlier" pages back to, 0 when there is nothing earlier
	Paged    bool         // an earlier page, not the latest Messages
	Body     string       // the text of a refused Message, kept for another try
	ReplyTo  *chatMessage // the Message the composer replies to, nil when none
}

// chatViewer is who is reading a Chat.
type chatViewer struct {
	*learner
	lang string
}

// chatFor loads the Course named by {id} and checks that the logged-in Learner may use its Chat: enrolled in it, or
// the Admin. When not, it writes the refusal and returns false.
func (a *App) chatFor(w http.ResponseWriter, r *http.Request) (course, chatViewer, bool) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return c, chatViewer{}, false
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
	if !c.Published && !l.IsAdmin {
		a.courseNotFound(w, r)
		return c, chatViewer{}, false
	}
	allowed, err := a.mayChat(r.Context(), l, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return c, chatViewer{}, false
	}
	if !allowed {
		a.render(w, r, http.StatusForbidden, "members_only", page{Learner: l, Course: &c, Message: text.T(lang, "chat.join-to-open")})
		return c, chatViewer{}, false
	}
	return c, chatViewer{l, lang}, true
}

// mayChat reports whether l may read and post in Course courseID's Chat: the Admin, or a Learner enrolled in it.
func (a *App) mayChat(ctx context.Context, l *learner, courseID int64) (bool, error) {
	if l.IsAdmin {
		return true, nil
	}
	return a.enrolled(ctx, l.ID, courseID)
}

// chatColumns selects a Message as viewer $1, who is the Admin when $2, sees it. Removed text never leaves the database.
const chatColumns = `m.id, m.course_id, l.name, l.is_admin, m.learner_id = $1,
	CASE WHEN m.hidden AND NOT $2 THEN '' ELSE m.body END, m.created_at, m.deleted, m.hidden,
	coalesce(m.reply_to, 0), o.id IS NULL OR o.deleted OR o.hidden,
	CASE WHEN o.deleted OR o.hidden THEN '' ELSE coalesce(ol.name, '') END,
	CASE WHEN o.deleted OR o.hidden THEN '' ELSE coalesce(split_part(o.body, E'\n', 1), '') END,
	EXISTS (SELECT 1 FROM chat_reports rp WHERE rp.message_id = m.id AND rp.learner_id = $1)
	FROM chat_messages m JOIN learners l ON l.id = m.learner_id
	LEFT JOIN chat_messages o ON o.id = m.reply_to LEFT JOIN learners ol ON ol.id = o.learner_id`

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
			&m.ReplyID, &m.ReplyRemoved, &m.ReplyWriter, &m.ReplyLine, &m.Reported); err != nil {
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
	list, err := a.chatMessages(r.Context(), v, `m.course_id = $3 AND m.id = $4 AND NOT m.deleted AND NOT m.hidden`, c.ID, id)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	if len(list) == 0 {
		return nil, true
	}
	return &list[0], true
}

// renderChat renders the Chat page p of Course c for v, loading its Messages.
func (a *App) renderChat(w http.ResponseWriter, r *http.Request, status int, c course, v chatViewer, p chatPage) {
	before := int64(math.MaxInt64)
	if b, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64); err == nil {
		before, p.Paged = b, true
	}
	list, err := a.chatMessages(r.Context(), v, `m.course_id = $3 AND m.id < $4 ORDER BY m.id DESC LIMIT $5`, c.ID, before, chatPageSize+1)
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
	case !v.IsAdmin && !a.limiter.allow(fmt.Sprintf("chat message %d", v.ID), messagesPerMinute, time.Minute, a.now()):
		refuse(http.StatusTooManyRequests, "chat.err.too-fast")
		return
	}
	var id int64
	var replyID *int64
	if replyTo != nil {
		replyID = &replyTo.ID
	}
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO chat_messages (course_id, learner_id, body, reply_to)
		VALUES ($1, $2, $3, $4) RETURNING id`, c.ID, v.ID, body, replyID).Scan(&id)
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
	list, err := a.chatMessages(r.Context(), v, `m.course_id = $3 AND m.id = $4`, c.ID, mid)
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

// deleteMessage is a writer deleting their own Message: its text is emptied and it shows "Message deleted".
func (a *App) deleteMessage(w http.ResponseWriter, r *http.Request) {
	c, v, ok := a.chatFor(w, r)
	if !ok {
		return
	}
	m, ok := a.chatMessageFor(w, r, c, v)
	if !ok {
		return
	}
	if !m.Mine {
		if refuseInPlace(w, r, http.StatusForbidden, text.T(v.lang, "chat.not-yours")) {
			return
		}
		a.render(w, r, http.StatusForbidden, "message", page{Title: text.T(v.lang, "chat.not-yours.title"), Message: text.T(v.lang, "chat.not-yours")})
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE chat_messages SET deleted = true, body = '' WHERE id = $1`, m.ID); err != nil {
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
	refuse := func(status int, title, key string) {
		if !refuseInPlace(w, r, status, text.T(v.lang, key)) {
			a.render(w, r, status, "message", page{Title: text.T(v.lang, title), Message: text.T(v.lang, key)})
		}
	}
	emoji := r.PostFormValue("emoji")
	switch {
	case !slices.Contains(reactions, emoji) || !m.Visible():
		refuse(http.StatusUnprocessableEntity, "chat.no-reaction.title", "chat.no-reaction")
		return
	case !v.IsAdmin && !a.limiter.allow(fmt.Sprintf("chat reaction %d", v.ID), reactionsPerMinute, time.Minute, a.now()):
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
	refuse := func(status int, title, key string) {
		if !refuseInPlace(w, r, status, text.T(v.lang, key)) {
			a.render(w, r, status, "message", page{Title: text.T(v.lang, title), Message: text.T(v.lang, key)})
		}
	}
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	switch {
	case !m.Visible():
		refuse(http.StatusUnprocessableEntity, "chat.not-reported", "chat.err.report-removed")
		return
	case utf8.RuneCountInString(reason) > maxReportReason:
		refuse(http.StatusUnprocessableEntity, "chat.not-reported", "chat.err.reason-too-long")
		return
	case !v.IsAdmin && !a.limiter.allow(fmt.Sprintf("chat report %d", v.ID), reportsPerHour, time.Hour, a.now()):
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
		refuse(http.StatusConflict, "chat.not-reported", "chat.err.reported")
		return
	}
	a.answer(w, r, v, m)
}

// setHidden returns the handler with which the Admin hides or unhides a Message. Hiding closes every open Report on
// it. The form's back field "reports" returns to the Reports page, else to the Chat at the Message.
func (a *App) setHidden(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m chatMessage
		mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
		if err == nil {
			err = a.db.QueryRowContext(r.Context(), `UPDATE chat_messages SET hidden = $2 WHERE id = $1 RETURNING id, course_id`,
				mid, hidden).Scan(&m.ID, &m.CourseID)
		}
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
			a.notFound(w, r)
			return
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if hidden {
			if _, err := a.db.ExecContext(r.Context(), `UPDATE chat_reports SET closed_at = now()
				WHERE message_id = $1 AND closed_at IS NULL`, m.ID); err != nil {
				a.serverError(w, r, err)
				return
			}
		}
		a.changed(r.Context(), m, true)
		if r.PostFormValue("back") == "reports" {
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

// backToMessage redirects to the Chat page that shows m: the latest, or an earlier one ending with it.
func (a *App) backToMessage(w http.ResponseWriter, r *http.Request, m chatMessage) {
	var newer int
	err := a.db.QueryRowContext(r.Context(), `SELECT count(*) FROM (SELECT 1 FROM chat_messages WHERE course_id = $1 AND id > $2
		LIMIT $3) n`, m.CourseID, m.ID, chatPageSize).Scan(&newer)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	to := fmt.Sprintf("/courses/%d/chat#m%d", m.CourseID, m.ID)
	if newer >= chatPageSize {
		to = fmt.Sprintf("/courses/%d/chat?before=%d#m%d", m.CourseID, m.ID+1, m.ID)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}
