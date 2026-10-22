package app

import (
	"fmt"
	"net/http"
	"text/template/parse"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	languageCookie   = "lang"
	languageLifetime = 365 * 24 * 60 * 60 // seconds
)

// chrome is what render fills in for layout.html on every page: the page's language and its own address.
type chrome struct {
	Lang string
	Here string // path and query of this page, for the language switch to come back to
}

func (c *chrome) chromeField() *chrome { return c }

// language is the request's one language: the logged-in Learner's saved one, else the language cookie,
// else the first of Malay or English the browser asks for, else Malay.
func (a *App) language(r *http.Request) (string, error) {
	l, err := a.currentLearner(r)
	if err != nil {
		return "", err
	}
	if l != nil && text.Supported(l.Lang) {
		return l.Lang, nil
	}
	if c, err := r.Cookie(languageCookie); err == nil && text.Supported(c.Value) {
		return c.Value, nil
	}
	return text.FromAcceptLanguage(r.Header.Get("Accept-Language")), nil
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
		MaxAge:   languageLifetime,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// templateTextKeys lists every catalog key a template names in {{t .Lang "key"}} or {{tn .Lang "key" n}}.
func templateTextKeys() ([]string, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	var keys []string
	var walk func(parse.Node)
	walk = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, c := range n.Nodes {
					walk(c)
				}
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.IfNode:
			walk(&n.BranchNode)
		case *parse.RangeNode:
			walk(&n.BranchNode)
		case *parse.WithNode:
			walk(&n.BranchNode)
		case *parse.BranchNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.TemplateNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			if n != nil {
				for _, c := range n.Cmds {
					walk(c)
				}
			}
		case *parse.CommandNode:
			if id, ok := n.Args[0].(*parse.IdentifierNode); ok && (id.Ident == "t" || id.Ident == "tn") && len(n.Args) > 2 {
				if key, ok := n.Args[2].(*parse.StringNode); ok {
					keys = append(keys, key.Text)
				}
			}
			for _, c := range n.Args {
				walk(c)
			}
		}
	}
	for _, page := range pages {
		for _, t := range page.Templates() {
			if t.Tree != nil {
				walk(t.Tree.Root)
			}
		}
	}
	return keys, nil
}
