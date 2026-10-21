package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// maxUserMasterPrompt is how many characters a User master prompt may have.
const maxUserMasterPrompt = 8000

// saveUserMasterPrompt keeps a Pro Learner's User master prompt as pasted. There is one per CaveText account and it
// is never edited: with one saved, the Learner deletes it first.
func (a *App) saveUserMasterPrompt(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	var in struct{ Text string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "CaveText sent a User master prompt CAVELMS can't read.")
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSONError(w, http.StatusUnprocessableEntity, "Paste the User master prompt first.")
		return
	}
	if n := utf8.RuneCountInString(in.Text); n > maxUserMasterPrompt {
		writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("A User master prompt can have up to %s characters; this one has %s. Shorten it, then save.", thousands(maxUserMasterPrompt), thousands(n)))
		return
	}
	fail := func(err error) {
		a.logError(r, fmt.Errorf("save User master prompt: %w", err))
		writeJSONError(w, http.StatusInternalServerError, caveTextProblem)
	}
	c, err := learnerSnippetCap(r.Context(), a.db, l.ID)
	if err != nil {
		fail(err)
		return
	}
	if !c.Pro {
		writeJSONError(w, http.StatusForbidden, "The User master prompt comes with CaveText Pro.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `
		UPDATE cavetext_accounts SET user_master_prompt = $2 WHERE learner_id = $1 AND user_master_prompt IS NULL`, l.ID, in.Text)
	if err != nil {
		fail(err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		fail(err)
		return
	}
	if n == 0 {
		writeJSONError(w, http.StatusConflict, "You already have a User master prompt. Delete it first to save a new one.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"userMasterPrompt": in.Text})
}

// deleteUserMasterPrompt always works, with Pro or without, and with none saved.
func (a *App) deleteUserMasterPrompt(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_accounts SET user_master_prompt = NULL WHERE learner_id = $1`, l.ID); err != nil {
		a.logError(r, fmt.Errorf("delete User master prompt: %w", err))
		writeJSONError(w, http.StatusInternalServerError, caveTextProblem)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
