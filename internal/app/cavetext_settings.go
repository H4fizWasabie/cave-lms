package app

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// caveTextSettingsView is the CaveText settings page any Admin edits: the question shown beside the User master
// prompt box; and, in a form of their own, the Image and Dashboard master prompts, with how many Learners have a
// Snippet clashing with their shortcuts.
type caveTextSettingsView struct {
	pageBase
	Question string

	Prompts                      masterPrompts
	PromptsError, PromptsMessage string
	Clashing                     int
}

// maxQuestion bounds the question's length in characters.
const maxQuestion = 2000

// MaxQuestion is maxQuestion for the page's maxlength.
func (caveTextSettingsView) MaxQuestion() int { return maxQuestion }

// caveTextSettings reads the page as saved; a refused save then puts back what the Admin posted.
func (a *App) caveTextSettings(r *http.Request) (*caveTextSettingsView, error) {
	v := &caveTextSettingsView{Prompts: newMasterPrompts()}
	if err := a.db.QueryRowContext(r.Context(), `SELECT s.master_prompt_question, `+masterPromptColumns+`
		FROM cavetext_settings s WHERE id = 1`).Scan(append([]any{&v.Question}, v.Prompts.dest()...)...); err != nil {
		return nil, fmt.Errorf("read CaveText settings: %w", err)
	}
	n, err := clashingLearners(r.Context(), a.db, v.Prompts)
	if err != nil {
		return nil, fmt.Errorf("count Snippets clashing with the master prompts: %w", err)
	}
	v.Clashing = n
	return v, nil
}

func (a *App) caveTextSettingsForm(w http.ResponseWriter, r *http.Request) {
	v, err := a.caveTextSettings(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if r.URL.Query().Has("saved") {
		v.Message = text.T(lang, "admin.common.saved")
	}
	if r.URL.Query().Has("prompts-saved") {
		v.PromptsMessage = text.T(lang, "ctadmin.prompts.saved")
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_settings", v)
}

func (a *App) saveCaveTextSettings(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	v := &caveTextSettingsView{Question: strings.TrimSpace(r.PostFormValue("question"))}
	switch {
	case v.Question == "":
		v.Error = text.T(lang, "ctadmin.settings.err.question")
	case utf8.RuneCountInString(v.Question) > maxQuestion:
		v.Error = text.T(lang, "ctadmin.settings.err.question-long", thousands(maxQuestion))
	}
	if v.Error != "" {
		saved, err := a.caveTextSettings(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		saved.Question, saved.Error = v.Question, v.Error
		a.render(w, r, http.StatusUnprocessableEntity, "admin_cavetext_settings", saved)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_settings SET master_prompt_question = $1 WHERE id = 1`, v.Question); err != nil {
		a.serverError(w, r, fmt.Errorf("save CaveText settings: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext/settings?saved", http.StatusSeeOther)
}

// parseRM reads an amount in RM with at most two decimals, like 25 or 25.50, as sen.
func parseRM(s string) (int, bool) {
	whole, frac, dotted := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 || dotted && frac == "" || strings.Trim(whole+frac, "0123456789") != "" {
		return 0, false
	}
	frac += strings.Repeat("0", 2-len(frac))
	sen, err := strconv.Atoi(whole + frac)
	return sen, err == nil && sen <= math.MaxInt32
}

// rm shows sen as RM: RM25, or RM25.50 when there are sen.
func rm(sen int) string {
	if sen%100 == 0 {
		return fmt.Sprintf("RM%d", sen/100)
	}
	return fmt.Sprintf("RM%d.%02d", sen/100, sen%100)
}
