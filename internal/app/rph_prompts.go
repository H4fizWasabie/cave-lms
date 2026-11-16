package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The RPH prompt (ADR 0029): a Teacher ticks SPs under one SK of a DSKP they can open, types the lesson's details and gets
// a prompt built from a fixed template (rph_template.go) to copy into their own AI chatbot. No AI call is made. Each
// prompt made is recorded as who and when, never what, for the Daily prompt limit: no history of prompts exists.
const (
	dailyPromptLimit = 3    // of each kind: RPH prompts and OPR prompts have a limit of their own
	maxRPHField      = 60   // time and class, in characters
	maxRPHStudents   = 200  // the number of murid
	maxRPHNotes      = 1000 // the optional notes, in characters
)

type rphSP struct {
	ID         int64
	Code, Text string
	Checked    bool
}

// rphSK is a Standard Kandungan with its tema or bidang and tajuk, and the SPs a Teacher may tick under it.
type rphSK struct {
	Group, Topic, Code, Text string
	SPs                      []rphSP
}

// rphPage is the RPH prompt form, and the prompt it makes.
type rphPage struct {
	pageBase
	Section                                      string
	Access                                       teacherAccess
	DSKP                                         dskp
	SKs                                          []rphSK
	Left                                         int    // RPH prompts still to make today
	Date, Time, Class, Students, Notes, Language string // the form as typed
	Prompt, Other                                string // the prompt made, and the same in the other language
	PromptLanguage, OtherLanguage                string
	SnippetName                                  string // what the prompt is called as a Snippet, to save it
}

// openableDSKP is the DSKP a Teacher may make an RPH prompt from, with its SKs and SPs and their ids; ok is false when
// there is none they can open. It is the one lookup of which DSKPs those are: the published ones of the library, and the
// Teacher's own uploads once read (a draft, however unchecked); one still being read or failed has no standards.
func (a *App) openableDSKP(ctx context.Context, learnerID, id int64) (d dskp, sks []rphSK, ok bool, err error) {
	d, groups, ok, err := a.libraryDSKP(ctx, id, dskpPublished)
	if err == nil && !ok {
		d, groups, ok, err = a.dskpOf(ctx, sql.NullInt64{Int64: learnerID, Valid: true}, id, dskpDraft)
	}
	if err != nil || !ok {
		return d, nil, false, err
	}
	for _, g := range groups {
		for _, t := range g.Topics {
			for _, sk := range t.SKs {
				r := rphSK{Group: g.Name, Topic: t.Title, Code: sk.Code, Text: sk.Text, SPs: make([]rphSP, len(sk.SPs))}
				for i, sp := range sk.SPs {
					r.SPs[i] = rphSP{ID: sp.ID, Code: sp.Code, Text: sp.Text}
				}
				sks = append(sks, r)
			}
		}
	}
	return d, sks, true, nil
}

// malaysiaDayStart is the start of the day, Malaysia time, that now falls in.
func malaysiaDayStart(now time.Time) time.Time {
	t := now.In(malaysia)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, malaysia)
}

// promptKind is which prompt was made: an RPH prompt or an OPR prompt, each with its own Daily prompt limit.
type promptKind string

const (
	promptRPH promptKind = "rph"
	promptOPR promptKind = "opr"
)

// promptsToday is how many prompts of a kind the Teacher has made since the day began, Malaysia time, in now's day.
func promptsToday(ctx context.Context, q rowQueryer, learnerID int64, kind promptKind, now time.Time) (made int, err error) {
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM prompts_made WHERE learner_id = $1 AND kind = $2 AND made_at >= $3`,
		learnerID, kind, malaysiaDayStart(now)).Scan(&made); err != nil {
		return 0, fmt.Errorf("count %s prompts: %w", kind, err)
	}
	return made, nil
}

// promptsLeft is how many prompts of a kind the Teacher may still make today, Malaysia time.
func (a *App) promptsLeft(ctx context.Context, learnerID int64, kind promptKind) (int, error) {
	made, err := promptsToday(ctx, a.db, learnerID, kind, a.now())
	return max(0, dailyPromptLimit-made), err
}

// rphForm is the RPH prompt form of a DSKP.
func (a *App) rphForm(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d, sks, ok, err := a.openableDSKP(r.Context(), l.ID, id)
	switch {
	case err != nil:
		a.serverError(w, r, err)
	case !ok:
		a.notFound(w, r)
	default:
		p := &rphPage{Access: acc, DSKP: d, SKs: sks, Date: a.now().In(malaysia).Format(time.DateOnly), Language: d.Language}
		// The time, class and number of murid last typed, kept on the Teacher profile; the Admin may have no profile.
		var students int
		err := a.db.QueryRowContext(r.Context(), `SELECT last_time, last_class, last_students FROM teacher_profiles WHERE learner_id = $1`, l.ID).Scan(&p.Time, &p.Class, &students)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, fmt.Errorf("read last lesson details: %w", err))
			return
		}
		if students > 0 {
			p.Students = strconv.Itoa(students)
		}
		a.renderRPH(w, r, http.StatusOK, p)
	}
}

// renderRPH fills in what every RPH page shows: the portal's section, the Teacher's access and the prompts left today.
func (a *App) renderRPH(w http.ResponseWriter, r *http.Request, status int, p *rphPage) {
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Section = "rph"
	if p.Left, err = a.promptsLeft(r.Context(), l.ID, promptRPH); err != nil {
		a.serverError(w, r, err)
		return
	}
	name := "teachers_rph"
	if p.Prompt != "" {
		name = "teachers_rph_prompt"
	}
	a.render(w, r, status, name, p)
}

// recordPrompt records one more prompt of a kind made by the Teacher, now, unless they have made the day's limit already; ok is
// false then. The count and the record are one step under the account's row, so two at once cannot both take the last place.
// keep, if given, runs in the same step once the prompt is recorded, so a prompt is never counted without it.
func (a *App) recordPrompt(ctx context.Context, learnerID int64, kind promptKind, keep func(*sql.Tx) error) (ok bool, err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("record %s prompt: %w", kind, err)
	}
	defer tx.Rollback()
	now := a.now()
	var locked int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learnerID).Scan(&locked); err != nil {
		return false, fmt.Errorf("record %s prompt: %w", kind, err)
	}
	made, err := promptsToday(ctx, tx, learnerID, kind, now)
	if err != nil {
		return false, err
	}
	if made >= dailyPromptLimit {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prompts_made (learner_id, kind, made_at) VALUES ($1, $2, $3)`, learnerID, kind, now); err != nil {
		return false, fmt.Errorf("record %s prompt: %w", kind, err)
	}
	if keep != nil {
		if err := keep(tx); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// makeRPHPrompt builds the RPH prompt from what the Teacher ticked and typed, and counts it as one of the day's.
func (a *App) makeRPHPrompt(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d, sks, ok, err := a.openableDSKP(r.Context(), l.ID, id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		a.notFound(w, r)
		return
	}
	p := &rphPage{Access: acc, DSKP: d, SKs: sks, Date: r.PostFormValue("date"), Time: clean(r.PostFormValue("time")), Class: clean(r.PostFormValue("class")),
		Students: strings.TrimSpace(r.PostFormValue("students")), Notes: strings.TrimSpace(r.PostFormValue("notes")), Language: r.PostFormValue("language")}
	if p.Language != "ms" && p.Language != "en" {
		p.Language = d.Language
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The SPs ticked: only this DSKP's count, and all must be under one SK.
	var lesson rphLesson
	picked, problem := -1, "" // the index of the SK the ticked SPs are under
	for i := range sks {
		for j := range sks[i].SPs {
			if !slices.Contains(r.PostForm["sp"], strconv.FormatInt(sks[i].SPs[j].ID, 10)) {
				continue
			}
			sks[i].SPs[j].Checked = true
			if picked != -1 && picked != i {
				problem = "teachers.rph.err.one-sk"
			}
			picked = i
			lesson.SPs = append(lesson.SPs, sks[i].SPs[j])
		}
	}
	date, dateErr := time.ParseInLocation(time.DateOnly, p.Date, malaysia)
	students, studentsErr := strconv.Atoi(p.Students)
	switch {
	case picked == -1:
		problem = "teachers.rph.err.no-sp"
	case problem != "":
	case dateErr != nil:
		problem = "teachers.rph.err.date"
	case p.Time == "" || utf8.RuneCountInString(p.Time) > maxRPHField:
		problem = "teachers.rph.err.time"
	case p.Class == "" || utf8.RuneCountInString(p.Class) > maxRPHField:
		problem = "teachers.rph.err.class"
	case studentsErr != nil || students < 1 || students > maxRPHStudents:
		problem = "teachers.rph.err.students"
	case utf8.RuneCountInString(p.Notes) > maxRPHNotes:
		problem = "teachers.rph.err.notes"
	}
	if problem != "" {
		p.Error = text.T(lang, problem)
		a.renderRPH(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	made, err := a.recordPrompt(r.Context(), l.ID, promptRPH, nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !made {
		p.Error = text.T(lang, "teachers.rph.err.limit", dailyPromptLimit, text.Date(lang, malaysiaDayStart(a.now()).AddDate(0, 0, 1)))
		a.renderRPH(w, r, http.StatusTooManyRequests, p)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE teacher_profiles SET last_time = $2, last_class = $3, last_students = $4 WHERE learner_id = $1`,
		l.ID, p.Time, p.Class, students); err != nil {
		a.serverError(w, r, fmt.Errorf("keep last lesson details: %w", err))
		return
	}
	sk := sks[picked]
	lesson.Subject, lesson.Year, lesson.Date, lesson.Time, lesson.Class, lesson.Students, lesson.Notes = d.Subject, d.Year, date, p.Time, p.Class, students, p.Notes
	lesson.Group, lesson.Topic, lesson.SKCode, lesson.SKText = sk.Group, sk.Topic, sk.Code, sk.Text
	p.PromptLanguage, p.OtherLanguage = p.Language, "en"
	if p.Language == "en" {
		p.OtherLanguage = "ms"
	}
	p.Prompt, p.Other = rphPromptText(p.PromptLanguage, lesson), rphPromptText(p.OtherLanguage, lesson)
	p.SnippetName = "RPH " + d.Subject + " SK " + sk.Code
	a.renderRPH(w, r, http.StatusOK, p)
}

// clean is typed text with its spaces and line breaks collapsed to single spaces.
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
