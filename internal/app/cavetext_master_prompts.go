package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// caveTextMasterPrompt is the Image or the Dashboard master prompt: written for the client in CaveText snippet syntax,
// the same for every CaveText Pro Learner, and inserted as it is. A free Learner is sent its kind and name only.
type caveTextMasterPrompt struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Shortcut *string `json:"shortcut,omitempty"`
	Body     *string `json:"body,omitempty"`
}

// masterPromptColumns are the Image then the Dashboard master prompt's name, shortcut and body, over
// cavetext_settings s, scanned with masterPromptsDest.
const masterPromptColumns = `s.image_prompt_name, s.image_prompt_shortcut, s.image_prompt_body,
	s.dashboard_prompt_name, s.dashboard_prompt_shortcut, s.dashboard_prompt_body`

// masterPrompts holds the two in order, Image first; Label is how the Admin page names each.
type masterPrompts [2]struct{ Kind, Label, Name, Shortcut, Body string }

func newMasterPrompts() masterPrompts {
	var p masterPrompts
	p[0].Kind, p[0].Label = "image", "Image"
	p[1].Kind, p[1].Label = "dashboard", "Dashboard"
	return p
}

func (p *masterPrompts) dest() []any {
	return []any{&p[0].Name, &p[0].Shortcut, &p[0].Body, &p[1].Name, &p[1].Shortcut, &p[1].Body}
}

// forLearner is what me sends: in full with Pro, otherwise the names for the locked view.
func (p *masterPrompts) forLearner(pro bool) []caveTextMasterPrompt {
	out := make([]caveTextMasterPrompt, len(p))
	for i := range p {
		out[i] = caveTextMasterPrompt{Kind: p[i].Kind, Name: p[i].Name}
		if pro {
			out[i].Shortcut, out[i].Body = &p[i].Shortcut, &p[i].Body
		}
	}
	return out
}

// reserved is the two shortcuts as Snippets for the clash check, so a Snippet's message names the master prompt.
func (p *masterPrompts) reserved() []caveTextSnippet {
	return []caveTextSnippet{{Name: p[0].Name, Shortcut: p[0].Shortcut}, {Name: p[1].Name, Shortcut: p[1].Shortcut}}
}

func readMasterPrompts(ctx context.Context, q rowQueryer) (masterPrompts, error) {
	p := newMasterPrompts()
	err := q.QueryRowContext(ctx, `SELECT `+masterPromptColumns+` FROM cavetext_settings s WHERE id = 1`).Scan(p.dest()...)
	return p, err
}

// clashList is what a Snippet's shortcut is checked against: the master prompts' shortcuts, then others, in a new
// slice (never reserved's backing array) with room for `room` more, so a caller checking a set one by one can grow it.
func clashList(reserved, others []caveTextSnippet, room int) []caveTextSnippet {
	list := make([]caveTextSnippet, 0, len(reserved)+len(others)+room)
	return append(append(list, reserved...), others...)
}

// reservedShortcuts are the master prompts' shortcuts, part of every Learner's clash check.
func reservedShortcuts(ctx context.Context, q rowQueryer) ([]caveTextSnippet, error) {
	p, err := readMasterPrompts(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("read master prompt shortcuts: %w", err)
	}
	return p.reserved(), nil
}

// Bounds on what the Admin saves, in characters.
const (
	maxMasterPromptName = 100
	maxMasterPromptBody = 20000
)

// MaxMasterPromptName and MaxMasterPromptBody are the bounds for the page's maxlength.
func (caveTextSettingsView) MaxMasterPromptName() int { return maxMasterPromptName }
func (caveTextSettingsView) MaxMasterPromptBody() int { return maxMasterPromptBody }

// problem says why the two can't be saved, or "". A shortcut follows the Snippet rules and can't clash with the other
// master prompt's; a clash with Learners' Snippets doesn't stop the save.
func (p *masterPrompts) problem() string {
	for i := range p {
		x := &p[i]
		var others []caveTextSnippet
		if i == 1 {
			others = []caveTextSnippet{{Name: p[0].Name, Shortcut: p[0].Shortcut}}
		}
		switch {
		case x.Name == "":
			return fmt.Sprintf("Enter a name for the %s master prompt.", x.Label)
		case utf8.RuneCountInString(x.Name) > maxMasterPromptName:
			return fmt.Sprintf("Shorten the %s master prompt's name to %d characters or fewer.", x.Label, maxMasterPromptName)
		case strings.TrimSpace(x.Body) == "":
			return fmt.Sprintf("Enter the %s master prompt.", x.Label)
		case utf8.RuneCountInString(x.Body) > maxMasterPromptBody:
			return fmt.Sprintf("Shorten the %s master prompt to %s characters or fewer.", x.Label, thousands(maxMasterPromptBody))
		}
		if why := shortcutProblem(x.Shortcut, others); why != "" {
			return fmt.Sprintf("%s master prompt: %s", x.Label, why)
		}
	}
	return ""
}

// clashingLearners counts the Learners with a Snippet whose shortcut clashes with a master prompt's. Their Snippets
// stay; editing one asks for another shortcut.
func clashingLearners(ctx context.Context, db *sql.DB, p masterPrompts) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT learner_id, shortcut FROM cavetext_snippets`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	reserved := p.reserved()
	learners := map[int64]bool{}
	for rows.Next() {
		var id int64
		var shortcut string
		if err := rows.Scan(&id, &shortcut); err != nil {
			return 0, err
		}
		if !learners[id] && shortcutProblem(shortcut, reserved) != "" {
			learners[id] = true
		}
	}
	return len(learners), rows.Err()
}

func (a *App) saveCaveTextMasterPrompts(w http.ResponseWriter, r *http.Request) {
	p := newMasterPrompts()
	for i := range p {
		k := p[i].Kind
		p[i].Name = strings.TrimSpace(r.PostFormValue(k + "_name"))
		p[i].Shortcut = strings.TrimSpace(r.PostFormValue(k + "_shortcut"))
		p[i].Body = strings.ReplaceAll(r.PostFormValue(k+"_body"), "\r\n", "\n")
	}
	if problem := p.problem(); problem != "" {
		v, err := a.caveTextSettings(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		v.Prompts, v.PromptsError = p, problem
		a.render(w, r, http.StatusUnprocessableEntity, "admin_cavetext_settings", v)
		return
	}
	args := []any{}
	for i := range p {
		args = append(args, p[i].Name, p[i].Shortcut, p[i].Body)
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_settings SET image_prompt_name = $1, image_prompt_shortcut = $2,
		image_prompt_body = $3, dashboard_prompt_name = $4, dashboard_prompt_shortcut = $5, dashboard_prompt_body = $6 WHERE id = 1`, args...); err != nil {
		a.serverError(w, r, fmt.Errorf("save master prompts: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext/settings?prompts-saved", http.StatusSeeOther)
}
