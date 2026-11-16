package app

import (
	"net/http"
	"strings"
)

// rphSnippetFolder is the CaveText folder an RPH prompt is saved in.
const rphSnippetFolder = "RPH"

// rphSnippetPage is the page that says an RPH prompt was saved as a Snippet, or why it was not, with the form to try again.
type rphSnippetPage struct {
	pageBase
	Section              string
	Access               teacherAccess
	Name, Shortcut, Body string
	Saved                bool
}

// saveRPHSnippet saves the prompt on screen as a Snippet in the Teacher's CaveText account, made if they have none yet,
// through the same Snippet creation CaveText uses, so the Snippet cap and the shortcut rules apply. It is not an RPH
// prompt made: the body is the text the Teacher was shown and sent back, never built again here, so the Daily prompt
// limit is neither asked nor touched.
func (a *App) saveRPHSnippet(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
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
	p := &rphSnippetPage{Section: "rph", Access: acc, Name: clean(r.PostFormValue("name")), Shortcut: strings.TrimSpace(r.PostFormValue("shortcut")), Body: r.PostFormValue("body")}
	if strings.TrimSpace(p.Body) == "" {
		a.notFound(w, r)
		return
	}
	if err := ensureCaveTextAccount(r.Context(), a.db, l.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	_, status, problem, err := a.createSnippet(r.Context(), &caveTextLearner{ID: l.ID, Name: l.Name, Lang: lang},
		caveTextSnippet{Folder: rphSnippetFolder, Name: p.Name, Shortcut: p.Shortcut, Body: p.Body, WithMasterPrompt: true})
	switch {
	case err != nil:
		a.serverError(w, r, err)
	case problem != "":
		p.Error = problem
		a.render(w, r, status, "teachers_rph_snippet", p)
	default:
		p.Saved = true
		a.render(w, r, http.StatusOK, "teachers_rph_snippet", p)
	}
}
