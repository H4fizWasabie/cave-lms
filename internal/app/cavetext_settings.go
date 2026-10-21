package app

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// caveTextSettingsView is the CaveText settings page any Admin edits: Pro's price, shown in RM, its payment link, and
// the question shown beside the User master prompt box; and, in a form of their own, the Image and Dashboard master
// prompts, with how many Learners have a Snippet clashing with their shortcuts.
type caveTextSettingsView struct {
	pageBase
	Price       string
	PaymentLink string
	Question    string

	Prompts                      masterPrompts
	PromptsError, PromptsMessage string
	Clashing                     int
}

// maxQuestion bounds the question's length in characters.
const maxQuestion = 2000

// MaxQuestion is maxQuestion for the page's maxlength.
func (caveTextSettingsView) MaxQuestion() int { return maxQuestion }

// maxProPriceSen bounds the price, so a typo can't ask a Learner for a fortune.
const maxProPriceSen = 9999999

// caveTextSettings reads the page as saved; a refused save then puts back what the Admin posted.
func (a *App) caveTextSettings(r *http.Request) (*caveTextSettingsView, error) {
	v := &caveTextSettingsView{pageBase: pageBase{Title: "CaveText settings"}, Prompts: newMasterPrompts()}
	var sen int
	if err := a.db.QueryRowContext(r.Context(), `SELECT s.pro_price_sen, s.pro_payment_link, s.master_prompt_question, `+masterPromptColumns+`
		FROM cavetext_settings s WHERE id = 1`).Scan(append([]any{&sen, &v.PaymentLink, &v.Question}, v.Prompts.dest()...)...); err != nil {
		return nil, fmt.Errorf("read CaveText settings: %w", err)
	}
	v.Price = strings.TrimPrefix(rm(sen), "RM")
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
	if r.URL.Query().Has("saved") {
		v.Message = "Saved."
	}
	if r.URL.Query().Has("prompts-saved") {
		v.PromptsMessage = "Master prompts saved."
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_settings", v)
}

func (a *App) saveCaveTextSettings(w http.ResponseWriter, r *http.Request) {
	v := &caveTextSettingsView{
		Price:       strings.TrimSpace(r.PostFormValue("price")),
		PaymentLink: strings.TrimSpace(r.PostFormValue("payment_link")),
		Question:    strings.TrimSpace(r.PostFormValue("question"))}
	sen, ok := parseRM(v.Price)
	switch {
	case !ok || sen > maxProPriceSen:
		v.Error = "Enter the price in RM, like 25 or 25.50."
	case sen == 0:
		v.Error = "Enter a price more than 0, like 25 or 25.50."
	case v.PaymentLink != "" && !isWebLink(v.PaymentLink):
		v.Error = "Enter the payment link as a full web address starting with http:// or https://, or leave it empty."
	case v.Question == "":
		v.Error = "Enter the question Learners ask an AI chatbot for their User master prompt."
	case utf8.RuneCountInString(v.Question) > maxQuestion:
		v.Error = fmt.Sprintf("Shorten the question to %s characters or fewer.", thousands(maxQuestion))
	}
	if v.Error != "" {
		saved, err := a.caveTextSettings(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		saved.Price, saved.PaymentLink, saved.Question, saved.Error = v.Price, v.PaymentLink, v.Question, v.Error
		a.render(w, r, http.StatusUnprocessableEntity, "admin_cavetext_settings", saved)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_settings SET pro_price_sen = $1, pro_payment_link = $2, master_prompt_question = $3 WHERE id = 1`,
		sen, v.PaymentLink, v.Question); err != nil {
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
