package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// teachersLengths is how long each Teachers Portal Pro product lasts, as a Postgres interval.
var teachersLengths = map[string]string{teachersMonthProduct: "1 month", teachersYearProduct: "12 months"}

// teachersGives are the {length} path values of the Admin's give, and the product each gives.
var teachersGives = map[string]string{"month": teachersMonthProduct, "year": teachersYearProduct}

// nextTeacherProStart is s.at, when the next Teachers Portal Pro period of the account $1 starts: now, or when their
// Teacher trial ends or their last period ends if that is later. A Payment and the Admin's give both start there, so a
// month or a year bought early loses no days and periods stack.
var nextTeacherProStart = `(SELECT GREATEST(now(),
	(SELECT trial_started_at + make_interval(days => ` + strconv.Itoa(teacherTrialDays) + `) FROM teacher_profiles WHERE learner_id = $1),
	(SELECT max(ends_at) FROM teacher_pro_periods WHERE learner_id = $1)) AS at) s`

// giveTeacherPro gives the account a month or a year of Teachers Portal Pro (product), given by "payment" or "admin",
// starting at nextTeacherProStart. It gives nothing else: no CaveText Pro and no Plan period.
// ponytail: a Payment and an Admin giving to one account at the same moment could overlap; fine at this volume.
func giveTeacherPro(ctx context.Context, db execer, learnerID int64, product, givenBy string) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO teacher_pro_periods (learner_id, product, given_by, starts_at, ends_at)
		SELECT $1, $2, $3, s.at, s.at + $4::interval FROM `+nextTeacherProStart,
		learnerID, product, givenBy, teachersLengths[product]); err != nil {
		return fmt.Errorf("give Teachers Portal Pro: %w", err)
	}
	return nil
}

// teacherProRow is one Teachers Portal Pro period on the Admin's page, in the page's language.
type teacherProRow struct{ Length, Starts, Ends, Given, Status string }

// teachersProPage is the Admin's Teachers Portal Pro page: the account searched for by email and its periods.
type teachersProPage struct {
	pageBase
	Email      string
	HolderID   int64
	HolderName string
	Periods    []teacherProRow
}

// adminTeachersPro shows the Teachers Portal Pro periods of the account whose email is searched for, with the gives.
func (a *App) adminTeachersPro(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &teachersProPage{Email: strings.TrimSpace(r.URL.Query().Get("email"))}
	status := http.StatusOK
	if p.Email != "" {
		err = a.db.QueryRowContext(r.Context(), `SELECT id, name FROM learners WHERE lower(email) = lower($1)`, p.Email).Scan(&p.HolderID, &p.HolderName)
		if errors.Is(err, sql.ErrNoRows) {
			status, p.Error, err = http.StatusNotFound, text.T(lang, "tpro.not-found", p.Email), nil
		} else if err == nil {
			p.Periods, err = a.teacherProRows(r.Context(), lang, p.HolderID)
		}
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "admin_teachers_pro", p)
}

// teacherProRows lists the account's periods in start order.
func (a *App) teacherProRows(ctx context.Context, lang string, learnerID int64) ([]teacherProRow, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT product, starts_at, ends_at, given_by,
			CASE WHEN ends_at <= now() THEN 'ended' WHEN starts_at > now() THEN 'upcoming' ELSE 'active' END
		FROM teacher_pro_periods WHERE learner_id = $1 ORDER BY starts_at, id`, learnerID)
	if err != nil {
		return nil, fmt.Errorf("list Teachers Portal Pro periods: %w", err)
	}
	defer rows.Close()
	var list []teacherProRow
	for rows.Next() {
		var product, givenBy, status string
		var starts, ends time.Time
		if err := rows.Scan(&product, &starts, &ends, &givenBy, &status); err != nil {
			return nil, err
		}
		list = append(list, teacherProRow{text.T(lang, "tpro."+product), dateTimeIn(lang, starts), dateTimeIn(lang, ends),
			text.T(lang, "tpro.given."+givenBy), text.T(lang, "plans.period."+status)})
	}
	return list, rows.Err()
}

// teacherProGive reads which account and product the give in the path names: {id} and {length}. When either is unknown
// it writes the Not found page and returns false.
func (a *App) teacherProGive(w http.ResponseWriter, r *http.Request) (id int64, name, email, product string, ok bool) {
	product, known := teachersGives[r.PathValue("length")]
	id, _ = strconv.ParseInt(r.PathValue("id"), 10, 64)
	if known {
		err := a.db.QueryRowContext(r.Context(), `SELECT name, email FROM learners WHERE id = $1`, id).Scan(&name, &email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return 0, "", "", "", false
		}
		known = err == nil
	}
	if !known {
		a.adminNotFound(w, r, "tpro.give-not-found.title", "tpro.give-not-found")
		return 0, "", "", "", false
	}
	return id, name, email, product, true
}

// confirmGiveTeacherPro asks the Admin to confirm giving a month or a year, naming when it starts and ends.
func (a *App) confirmGiveTeacherPro(w http.ResponseWriter, r *http.Request) {
	id, name, email, product, ok := a.teacherProGive(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var starts, ends time.Time
	if err := a.db.QueryRowContext(r.Context(), `SELECT s.at, s.at + $2::interval FROM `+nextTeacherProStart, id, teachersLengths[product]).Scan(&starts, &ends); err != nil {
		a.serverError(w, r, fmt.Errorf("read when Teachers Portal Pro would start: %w", err))
		return
	}
	a.confirm(w, r, http.StatusOK, confirmation{Side: "teachers-pro", Heading: "tpro.give." + r.PathValue("length"), Question: "tpro.give-ask", Solid: true,
		Cancel: teachersProPath(email), Action: r.URL.Path}, name, text.T(lang, "tpro.what."+product), text.Date(lang, starts.In(malaysia)), text.Date(lang, ends.In(malaysia)))
}

// giveTeacherProByHand gives the month or year, once its confirmation page is posted, then goes back to the account.
func (a *App) giveTeacherProByHand(w http.ResponseWriter, r *http.Request) {
	id, _, email, product, ok := a.teacherProGive(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if err := giveTeacherPro(r.Context(), a.db, id, product, "admin"); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, teachersProPath(email), http.StatusSeeOther)
}

// teachersProPath is the Admin's Teachers Portal Pro page of the account with email.
func teachersProPath(email string) string {
	return "/admin/teachers-pro?email=" + url.QueryEscape(email)
}
