package app

import (
	"net/http"
	"strings"
)

// promptSnippetKind is what differs between saving an RPH prompt and an OPR prompt as a Snippet: the portal section, the
// CaveText folder, where the Teacher goes next and the catalog keys the page's words come from (prefix + ".save.title", ...).
type promptSnippetKind struct {
	Section, Folder, Action, Prefix, Back, BackText string
}

var (
	rphSnippet = promptSnippetKind{Section: "rph", Folder: "RPH", Action: portalPath + "/rph/snippet", Prefix: "teachers.rph", Back: portalPath + "/dskps", BackText: "teachers.dskps.back"}
	oprSnippet = promptSnippetKind{Section: "opr", Folder: "OPR", Action: portalPath + "/opr/snippet", Prefix: "teachers.opr", Back: portalPath + "/opr", BackText: "teachers.opr.another"}
)

// promptSnippetPage is the page that says a prompt was saved as a Snippet, or why it was not, with the form to try again.
type promptSnippetPage struct {
	pageBase
	promptSnippetKind
	Access               teacherAccess
	Name, Shortcut, Body string
	Saved                bool
}

// saveSnippet saves the prompt on screen as a Snippet in the Teacher's CaveText account, made if they have none yet,
// through the same Snippet creation CaveText uses, so the Snippet cap and the shortcut rules apply. It is not a prompt
// made: the body is the text the Teacher was shown and sent back, never built again here, so the Daily prompt limit is
// neither asked nor touched.
func (k promptSnippetKind) saveSnippet(a *App) func(http.ResponseWriter, *http.Request, teacherAccess) {
	return func(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
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
		p := &promptSnippetPage{promptSnippetKind: k, Access: acc, Name: clean(r.PostFormValue("name")), Shortcut: strings.TrimSpace(r.PostFormValue("shortcut")), Body: r.PostFormValue("body")}
		if strings.TrimSpace(p.Body) == "" {
			a.notFound(w, r)
			return
		}
		if err := ensureCaveTextAccount(r.Context(), a.db, l.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		_, status, problem, err := a.createSnippet(r.Context(), &caveTextLearner{ID: l.ID, Name: l.Name, Lang: lang},
			caveTextSnippet{Folder: k.Folder, Name: p.Name, Shortcut: p.Shortcut, Body: p.Body, WithMasterPrompt: true})
		switch {
		case err != nil:
			a.serverError(w, r, err)
		case problem != "":
			p.Error = problem
			a.render(w, r, status, "teachers_prompt_snippet", p)
		default:
			p.Saved = true
			a.render(w, r, http.StatusOK, "teachers_prompt_snippet", p)
		}
	}
}
