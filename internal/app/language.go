package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	languageCookie   = "lang"
	languageLifetime = 365 * 24 * time.Hour
)

// chrome is what render fills in for layout.html on every page: the page's language, its own address
// and the header's Courses menu.
type chrome struct {
	Lang   string
	Here   string   // path and query of this page for the language switch to come back to, or "" for home
	Latest []course // the 3 newest open Courses
}

func (c *chrome) chromeField() *chrome { return c }

// newChrome is the header data for a page in lang. When the Courses menu cannot be loaded it is left
// empty and the error returned, so the error page can still show the rest of the header.
func (a *App) newChrome(r *http.Request, lang string) (chrome, error) {
	c := chrome{Lang: lang, Here: here(r)}
	latest, err := a.queryCourses(r, `SELECT `+courseColumns+` FROM courses WHERE is_open ORDER BY created_at DESC, id DESC LIMIT 3`)
	if err == nil {
		c.Latest = latest
	}
	return c, err
}

// here is where the language switch on a page comes back to. A page shown in answer to a form post
// sends it home instead, since its address may take only posts.
func here(r *http.Request) string {
	if r.Method != http.MethodGet {
		return ""
	}
	return r.URL.RequestURI()
}

// language is the request's one language: the logged-in Learner's saved one, else the language cookie,
// else the first of Malay or English the browser asks for, else Malay.
func (a *App) language(r *http.Request) (string, error) {
	l, err := a.currentLearner(r)
	if err != nil {
		return "", err
	}
	saved := ""
	if l != nil {
		saved = l.Lang
	}
	return requestLanguage(saved, r), nil
}

// requestLanguage is saved when it is a language Cave speaks, else the language cookie, else the browser's.
// The CaveText API passes the token's Learner's saved language; its requests carry no cookie.
func requestLanguage(saved string, r *http.Request) string {
	if text.Supported(saved) {
		return saved
	}
	if c, err := r.Cookie(languageCookie); err == nil && text.Supported(c.Value) {
		return c.Value
	}
	return text.FromAcceptLanguage(r.Header.Get("Accept-Language"))
}

// switchLanguage is the header's BM | EN form: it remembers the language in a cookie, saves it on the
// logged-in Learner's account, and goes back to the page the form was on.
func (a *App) switchLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.PostFormValue("lang")
	if !text.Supported(lang) {
		http.Error(w, "unknown language", http.StatusBadRequest)
		return
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if l != nil {
		if _, err := a.db.ExecContext(r.Context(), `UPDATE learners SET language = $1 WHERE id = $2`, lang, l.ID); err != nil {
			a.serverError(w, r, fmt.Errorf("save language: %w", err))
			return
		}
	}
	setLanguageCookie(w, lang)
	next := localPath(r.PostFormValue("next"))
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func setLanguageCookie(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:     languageCookie,
		Value:    lang,
		Path:     "/",
		MaxAge:   int(languageLifetime.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
