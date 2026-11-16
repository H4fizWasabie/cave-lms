package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The Teachers Portal lives under portalPath (ADR 0029): its own pages, layout and navigation, on the main host.
const (
	portalPath       = "/teachers"
	teacherTrialDays = 14
	maxSubjects      = 20
	maxSubjectLength = 60
)

// teacherYears are the years a Teacher may tick: Tahun 1 to 6 (Y1 to Y6) and Tingkatan 1 to 5 (F1 to F5).
var teacherYears = []string{"Y1", "Y2", "Y3", "Y4", "Y5", "Y6", "F1", "F2", "F3", "F4", "F5"}

// inPortal reports whether next is the portal or a page inside it.
func inPortal(next string) bool {
	u, err := url.Parse(next)
	return err == nil && (u.Path == portalPath || strings.HasPrefix(u.Path, portalPath+"/"))
}

// teacherState is what the portal's access check answers for an account at this moment.
type teacherState int

const (
	teacherNone   teacherState = iota // no Teacher profile: the portal asks for one
	teacherTrial                      // the Teacher trial is running
	teacherPro                        // a month or a year of Teachers Portal Pro is running
	teacherLapsed                     // the Teacher trial and any Teachers Portal Pro have ended
	teacherAdmin                      // the Admin, who needs no profile or trial
)

// teacherAccess is the portal's access check for one account: its state, its Teacher profile when it has one, the days
// left in the trial while it runs, when the trial ends, and when the last Teachers Portal Pro period, running or queued
// after the trial, ends (zero when there is none).
type teacherAccess struct {
	State     teacherState
	Profile   *teacherProfile
	DaysLeft  int
	TrialEnds time.Time
	ProUntil  time.Time
}

func (t teacherAccess) Trial() bool  { return t.State == teacherTrial }
func (t teacherAccess) Pro() bool    { return t.State == teacherPro }
func (t teacherAccess) Lapsed() bool { return t.State == teacherLapsed }
func (t teacherAccess) Admin() bool  { return t.State == teacherAdmin }

// teacherProfile is a Teacher's subjects (typed names joined by ", ") and years (codes in teacherYears order).
type teacherProfile struct {
	Subjects string
	Years    []string
}

// teacherAccessOf is the check every portal page goes through. The Admin passes without a profile or trial. An account
// with no Teacher profile is asked for one first, even with Teachers Portal Pro running.
func (a *App) teacherAccessOf(ctx context.Context, l *learner) (teacherAccess, error) {
	var (
		acc      teacherAccess
		p        teacherProfile
		years    string
		days     int
		proUntil sql.NullTime
		pro      bool
	)
	err := a.db.QueryRowContext(ctx, `
		SELECT subjects, years, ceil(extract(epoch FROM trial_started_at + make_interval(days => $2) - now()) / 86400)::int,
			trial_started_at + make_interval(days => $2)
		FROM teacher_profiles WHERE learner_id = $1`, l.ID, teacherTrialDays).Scan(&p.Subjects, &years, &days, &acc.TrialEnds)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		acc.State = teacherNone
	case err != nil:
		return acc, fmt.Errorf("look up Teacher profile: %w", err)
	default:
		p.Years = strings.Split(years, ",")
		acc.Profile = &p
		acc.State, acc.DaysLeft = teacherLapsed, 0
		if days > 0 {
			acc.State, acc.DaysLeft = teacherTrial, days
		}
		if err := a.db.QueryRowContext(ctx, `SELECT max(ends_at), COALESCE(bool_or(starts_at <= now()), false)
			FROM teacher_pro_periods WHERE learner_id = $1 AND ends_at > now()`, l.ID).Scan(&proUntil, &pro); err != nil {
			return acc, fmt.Errorf("look up Teachers Portal Pro: %w", err)
		}
		acc.ProUntil = proUntil.Time
		if pro {
			acc.State, acc.DaysLeft = teacherPro, 0
		}
	}
	if l.IsAdmin {
		acc.State = teacherAdmin
	}
	return acc, nil
}

// yearOption is one tick box of the Teacher profile form.
type yearOption struct {
	Code    string
	Checked bool
}

// portalPage is the data of every page in the Teachers Portal.
type portalPage struct {
	pageBase
	Section       string // "rph" or "profile": the current one in the portal's navigation
	Access        teacherAccess
	Offers        []offer      // Teachers Portal Pro by the month and by the year, on the public page and the pay page
	ProUntil      string       // when Teachers Portal Pro ends, in the page's language, while it runs
	Subjects      string       // the Teacher profile form's subjects as typed or saved
	YearOptions   []yearOption // and its years
	SubjectsError string
	YearsError    string
}

// teachersHome is the portal's address: the public page for a Visitor, the Teacher profile form for an Account holder
// without a profile, the portal for everyone else.
func (a *App) teachersHome(w http.ResponseWriter, r *http.Request) {
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
	var acc teacherAccess
	if l != nil {
		if acc, err = a.teacherAccessOf(r.Context(), l); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	p := &portalPage{Section: "rph", Access: acc}
	switch {
	case l == nil || acc.Lapsed():
		if p.Offers, err = a.offersOf(r.Context(), teachersMonthProduct, teachersYearProduct); err != nil {
			a.serverError(w, r, err)
			return
		}
		page := "teachers_pay"
		if l == nil {
			page = "teachers_public"
		}
		a.render(w, r, http.StatusOK, page, p)
		return
	case acc.State == teacherNone:
		http.Redirect(w, r, portalPath+"/profile", http.StatusSeeOther)
		return
	case acc.Pro():
		p.ProUntil = text.Date(lang, acc.ProUntil.In(malaysia))
	}
	a.render(w, r, http.StatusOK, "teachers_home", p)
}

// teacherProfileForm is the Teacher profile form, filled in with the saved profile.
func (a *App) teacherProfileForm(w http.ResponseWriter, r *http.Request) {
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	acc, err := a.teacherAccessOf(r.Context(), l)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if acc.Lapsed() { // only the pay page is left, and the profile is kept as it is
		http.Redirect(w, r, portalPath, http.StatusSeeOther)
		return
	}
	p := &portalPage{Section: "profile", Access: acc}
	if acc.Profile != nil {
		p.Subjects, p.YearOptions = acc.Profile.Subjects, yearOptions(acc.Profile.Years)
	} else {
		p.YearOptions = yearOptions(nil)
	}
	a.render(w, r, http.StatusOK, "teachers_profile", p)
}

func yearOptions(ticked []string) []yearOption {
	var opts []yearOption
	for _, code := range teacherYears {
		opts = append(opts, yearOption{code, slices.Contains(ticked, code)})
	}
	return opts
}

// saveTeacherProfile saves the subjects and years, which on a first save also starts the Teacher trial; later saves leave
// the trial's start alone, so an account has one trial.
func (a *App) saveTeacherProfile(w http.ResponseWriter, r *http.Request) {
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
	acc, err := a.teacherAccessOf(r.Context(), l)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if acc.Lapsed() {
		http.Redirect(w, r, portalPath, http.StatusSeeOther)
		return
	}
	subjects, subjectsOK := cleanSubjects(r.PostFormValue("subjects"))
	var years []string
	for _, code := range teacherYears { // in this order, and only these
		if slices.Contains(r.PostForm["years"], code) {
			years = append(years, code)
		}
	}
	if !subjectsOK || len(years) == 0 {
		p := &portalPage{Section: "profile", Access: acc, Subjects: r.PostFormValue("subjects"), YearOptions: yearOptions(years)}
		if !subjectsOK {
			p.SubjectsError = text.T(lang, "teachers.err.subjects")
		}
		if len(years) == 0 {
			p.YearsError = text.T(lang, "teachers.err.years")
		}
		a.render(w, r, http.StatusUnprocessableEntity, "teachers_profile", p)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `
		INSERT INTO teacher_profiles (learner_id, subjects, years) VALUES ($1, $2, $3)
		ON CONFLICT (learner_id) DO UPDATE SET subjects = EXCLUDED.subjects, years = EXCLUDED.years`,
		l.ID, subjects, strings.Join(years, ",")); err != nil {
		a.serverError(w, r, fmt.Errorf("save Teacher profile: %w", err))
		return
	}
	http.Redirect(w, r, portalPath, http.StatusSeeOther)
}

// cleanSubjects reads subjects typed one after another, split by commas or lines, without blanks or repeats (ignoring
// case), and returns them joined by ", ". ok is false when there are none, more than maxSubjects, or one too long.
func cleanSubjects(typed string) (joined string, ok bool) {
	var kept []string
	seen := map[string]bool{}
	for _, s := range strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		s = strings.Join(strings.Fields(s), " ")
		if s == "" || seen[strings.ToLower(s)] {
			continue
		}
		if len([]rune(s)) > maxSubjectLength {
			return "", false
		}
		seen[strings.ToLower(s)] = true
		kept = append(kept, s)
	}
	if len(kept) == 0 || len(kept) > maxSubjects {
		return "", false
	}
	return strings.Join(kept, ", "), true
}

// landingAfterLogin is where a login with no next goes: the portal for an account with a Teacher profile and no Course
// enrolment, running Plan period or running CaveText Pro, else the home page as always. The CaveText website has no portal.
func (a *App) landingAfterLogin(r *http.Request, learnerID int64) (string, error) {
	if a.onCaveTextHost(r) {
		return "/", nil
	}
	var teacherOnly bool
	err := a.db.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM teacher_profiles WHERE learner_id = $1)
			AND NOT EXISTS (SELECT 1 FROM enrollments WHERE learner_id = $1)
			AND NOT EXISTS (SELECT 1 FROM plan_periods pp WHERE pp.learner_id = $1 AND `+activePeriodSQL+`)
			AND NOT EXISTS (SELECT 1 FROM cavetext_pro_periods pp WHERE pp.learner_id = $1 AND `+activePeriodSQL+`)`, learnerID).Scan(&teacherOnly)
	if err != nil {
		return "", fmt.Errorf("look up landing page: %w", err)
	}
	if teacherOnly {
		return portalPath, nil
	}
	return "/", nil
}
