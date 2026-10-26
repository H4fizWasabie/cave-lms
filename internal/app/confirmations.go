package app

import (
	"net/http"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// confirmation is a confirmation page (confirm.html). Heading is the catalog key naming the action, on the page head
// and the button; Question is the key of the question naming the thing and what is lost. Cancel is where Cancel goes,
// labelled CancelLabel (Cancel when empty), and the button posts confirm=yes to Action. Side is the admin sidebar
// section; a Learner page has none. Error is the key of why the action was refused, which hides the button.
type confirmation struct {
	pageBase
	Side, Heading, Question, Cancel, CancelLabel, Action string
}

// confirm renders c with its question (filled with args) and error in the request's language.
func (a *App) confirm(w http.ResponseWriter, r *http.Request, status int, c confirmation, args ...any) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c.Question = text.T(lang, c.Question, args...)
	if c.Error != "" {
		c.Error = text.T(lang, c.Error)
	}
	a.render(w, r, status, "confirm", &c)
}

// unconfirmed sends a destructive post that did not come from its confirmation page to that page (the same address,
// asked for with GET) and reports true; it then does nothing.
func unconfirmed(w http.ResponseWriter, r *http.Request) bool {
	if confirmed(r) {
		return false
	}
	http.Redirect(w, r, r.URL.RequestURI(), http.StatusSeeOther)
	return true
}
