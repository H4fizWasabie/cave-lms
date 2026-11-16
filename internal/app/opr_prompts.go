package app

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The OPR prompt (spec #578): a Teacher types an OPR's details, picks one of the shown OPR styles and the OPR's language, and
// gets a prompt built from a fixed core (opr_template.go) and the style's text, to paste into their own AI chatbot with their
// reference attached there. CAVELMS takes no file and makes no AI call. Each prompt made is recorded as who and when, never
// what, for the Daily prompt limit of OPR prompts, apart from RPH prompts: no history of prompts exists.

// Bounds on what a Teacher types for an OPR, in characters.
const (
	maxOPRShort   = 200  // title, institution, venue, target group, reporting teacher and reference
	maxOPRField   = 100  // date, time and colour
	maxOPRExtract = 2000 // what to take from the reference
	maxOPRExtra   = 1000 // the additional instruction
)

// oprPage is the OPR prompt form, and the prompt it makes.
type oprPage struct {
	pageBase
	Section string
	Access  teacherAccess
	Styles  []oprStyle // the OPR styles offered: the shown ones
	Left    int        // OPR prompts still to make today
	// The form as typed, the OPR style picked by position and the OPR's language.
	Title, Institution, Date, Time, Place, Target, Reporter, Reference, Extract, Extra, Colour, Language string
	Style                                                                                                int
	Prompt                                                                                               string // the prompt made
}

// renderOPR fills in what every OPR page shows: the portal's section, the Teacher's access, the OPR styles offered and the
// OPR prompts left today.
func (a *App) renderOPR(w http.ResponseWriter, r *http.Request, status int, p *oprPage) {
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Section = "opr"
	if p.Styles == nil { // the handler may have read them already
		if p.Styles, err = a.shownOPRStyles(r.Context()); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Left, err = a.promptsLeft(r.Context(), l.ID, promptOPR); err != nil {
		a.serverError(w, r, err)
		return
	}
	name := "teachers_opr"
	if p.Prompt != "" {
		name = "teachers_opr_prompt"
	}
	a.render(w, r, status, name, p)
}

// oprForm is the OPR prompt form, in the page's language.
func (a *App) oprForm(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderOPR(w, r, http.StatusOK, &oprPage{Access: acc, Language: lang})
}

// makeOPRPrompt builds the OPR prompt from what the Teacher typed and counts it as one of the day's.
func (a *App) makeOPRPrompt(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &oprPage{Access: acc, Title: clean(r.PostFormValue("title")), Institution: clean(r.PostFormValue("institution")), Date: clean(r.PostFormValue("date")),
		Time: clean(r.PostFormValue("time")), Place: clean(r.PostFormValue("place")), Target: clean(r.PostFormValue("target")),
		Reporter: clean(r.PostFormValue("reporter")), Reference: clean(r.PostFormValue("reference")), Extract: strings.TrimSpace(r.PostFormValue("extract")),
		Extra: strings.TrimSpace(r.PostFormValue("extra")), Colour: clean(r.PostFormValue("colour")), Language: r.PostFormValue("language")}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Language != "ms" && p.Language != "en" {
		p.Language = lang
	}
	p.Style, _ = strconv.Atoi(r.PostFormValue("style"))
	if p.Styles, err = a.shownOPRStyles(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(p.Styles) == 0 { // none is ready
		a.renderOPR(w, r, http.StatusServiceUnavailable, p)
		return
	}
	for _, f := range []struct {
		value, key string
		max        int
		required   bool
	}{
		{p.Title, "title", maxOPRShort, true}, {p.Institution, "institution", maxOPRShort, true}, {p.Date, "date", maxOPRField, true},
		{p.Place, "place", maxOPRShort, true}, {p.Reporter, "reporter", maxOPRShort, true}, {p.Reference, "reference", maxOPRShort, true},
		{p.Extract, "extract", maxOPRExtract, true}, {p.Time, "time", maxOPRField, false}, {p.Target, "target", maxOPRShort, false},
		{p.Extra, "extra", maxOPRExtra, false}, {p.Colour, "colour", maxOPRField, false},
	} {
		if (f.required && f.value == "") || utf8.RuneCountInString(f.value) > f.max {
			p.Error = text.T(lang, "teachers.opr.err."+f.key)
			a.renderOPR(w, r, http.StatusUnprocessableEntity, p)
			return
		}
	}
	i := slices.IndexFunc(p.Styles, func(s oprStyle) bool { return s.Position == p.Style })
	if i < 0 { // a hidden or unknown style is not offered, so it is not made
		p.Error = text.T(lang, "teachers.opr.err.style")
		a.renderOPR(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	style := p.Styles[i]
	made, err := a.recordPrompt(r.Context(), l.ID, promptOPR)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !made {
		p.Error = text.T(lang, "teachers.opr.err.limit", dailyPromptLimit, text.Date(lang, malaysiaDayStart(a.now()).AddDate(0, 0, 1)))
		a.renderOPR(w, r, http.StatusTooManyRequests, p)
		return
	}
	p.Prompt = oprPromptText(p.Language, oprDetails{Title: p.Title, Institution: p.Institution, Date: p.Date, Time: p.Time, Place: p.Place, Target: p.Target,
		Reporter: p.Reporter, Reference: p.Reference, Extract: p.Extract, Extra: p.Extra, Colour: p.Colour}, style)
	a.renderOPR(w, r, http.StatusOK, p)
}
