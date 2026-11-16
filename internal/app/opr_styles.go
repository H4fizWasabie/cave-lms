package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// oprStyle is one of the five OPR styles: slot Position (1 to 5), a Name the Teacher picks by, the Text of the style that goes
// into the OPR prompt, and whether the Admin has hidden it.
type oprStyle struct {
	Position int
	Name     string
	Text     string
	Hidden   bool
}

const oprStyleColumns = `position, name, prompt, hidden`

func scanOPRStyle(row interface{ Scan(...any) error }) (oprStyle, error) {
	var s oprStyle
	err := row.Scan(&s.Position, &s.Name, &s.Text, &s.Hidden)
	return s, err
}

// oprStyles lists the OPR styles matching where in position order.
func (a *App) oprStyles(ctx context.Context, where string) ([]oprStyle, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+oprStyleColumns+` FROM opr_styles `+where+` ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []oprStyle
	for rows.Next() {
		s, err := scanOPRStyle(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// shownOPRStyles are the OPR styles a Teacher is offered, in position order: none of the hidden ones.
func (a *App) shownOPRStyles(ctx context.Context) ([]oprStyle, error) {
	return a.oprStyles(ctx, `WHERE NOT hidden`)
}

// oprStyleAt is the OPR style in slot position, hidden or not (a caller offering styles to a Teacher checks Hidden); ok is
// false when there is no such slot.
func (a *App) oprStyleAt(ctx context.Context, position int) (s oprStyle, ok bool, err error) {
	s, err = scanOPRStyle(a.db.QueryRowContext(ctx, `SELECT `+oprStyleColumns+` FROM opr_styles WHERE position = $1`, position))
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}

// Bounds on an OPR style's name and prompt text, in characters.
const (
	maxOPRStyleName = 80
	maxOPRStyleText = 4000
)

type oprStyleFormPage struct {
	pageBase
	Style oprStyle
}

// MaxName and MaxText are the length limits, for the form's maxlength.
func (oprStyleFormPage) MaxName() int { return maxOPRStyleName }
func (oprStyleFormPage) MaxText() int { return maxOPRStyleText }

// oprStyleFor loads the OPR style named by the {position} path value. When there is none it writes the Not found page and
// returns false.
func (a *App) oprStyleFor(w http.ResponseWriter, r *http.Request) (oprStyle, bool) {
	position, err := strconv.Atoi(r.PathValue("position"))
	if err != nil {
		a.adminNotFound(w, r, "opr.styles.not-found.title", "opr.styles.not-found")
		return oprStyle{}, false
	}
	s, ok, err := a.oprStyleAt(r.Context(), position)
	if err != nil {
		a.serverError(w, r, err)
		return s, false
	}
	if !ok {
		a.adminNotFound(w, r, "opr.styles.not-found.title", "opr.styles.not-found")
	}
	return s, ok
}

func (a *App) editOPRStyleForm(w http.ResponseWriter, r *http.Request) {
	if s, ok := a.oprStyleFor(w, r); ok {
		a.render(w, r, http.StatusOK, "admin_opr_style", &oprStyleFormPage{Style: s})
	}
}

// saveOPRStyle stores the form's name and text in the slot.
func (a *App) saveOPRStyle(w http.ResponseWriter, r *http.Request) {
	s, ok := a.oprStyleFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	s.Name, s.Text = strings.TrimSpace(r.PostFormValue("name")), strings.TrimSpace(r.PostFormValue("text"))
	var failure string
	switch {
	case s.Name == "":
		failure = text.T(lang, "opr.styles.err.name")
	case utf8.RuneCountInString(s.Name) > maxOPRStyleName:
		failure = text.T(lang, "opr.styles.err.name-long", maxOPRStyleName)
	case s.Text == "":
		failure = text.T(lang, "opr.styles.err.text")
	case utf8.RuneCountInString(s.Text) > maxOPRStyleText:
		failure = text.T(lang, "opr.styles.err.text-long", thousands(maxOPRStyleText))
	}
	if failure != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "admin_opr_style", &oprStyleFormPage{pageBase{Error: failure}, s})
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE opr_styles SET name = $2, prompt = $3 WHERE position = $1`, s.Position, s.Name, s.Text); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/opr-styles", http.StatusSeeOther)
}

// setOPRStyleHidden hides or shows the slot's OPR style.
func (a *App) setOPRStyleHidden(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.oprStyleFor(w, r)
		if !ok {
			return
		}
		if _, err := a.db.ExecContext(r.Context(), `UPDATE opr_styles SET hidden = $2 WHERE position = $1`, s.Position, hidden); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/admin/opr-styles", http.StatusSeeOther)
	}
}

type oprStylesPage struct {
	pageBase
	Styles []oprStyle
}

// adminOPRStyles lists the five OPR styles with whether each is hidden.
func (a *App) adminOPRStyles(w http.ResponseWriter, r *http.Request) {
	list, err := a.oprStyles(r.Context(), "")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_opr_styles", &oprStylesPage{Styles: list})
}
