package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// caveTextTokenLifetime is how long a CaveText token lasts; each use starts it again.
const caveTextTokenLifetime = "1 year"

// caveTextConnect is where the extension's sign-in window opens. A Visitor logs in first and comes back here;
// a Learner gets a CaveText account on their first visit, then a new token, sent to the extension's own
// redirect URL. Only extensions listed in CAVETEXT_EXTENSION_IDS get one, so a token can't be sent elsewhere.
func (a *App) caveTextConnect(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("extension")
	if id == "" || !slices.Contains(a.caveTextExtensionIDs, id) {
		a.render(w, r, http.StatusBadRequest, "message", page{Title: "Unknown extension", Message: "This extension can't sign in to CAVELMS. Install CaveText from the Chrome Web Store."})
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if learner == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	// The NOT EXISTS keeps a repeat sign-in from using up a place number; only two first sign-ins racing skip one.
	if _, err := a.db.ExecContext(r.Context(), `
		INSERT INTO cavetext_accounts (learner_id) SELECT $1
		WHERE NOT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)
		ON CONFLICT DO NOTHING`, learner.ID); err != nil {
		a.serverError(w, r, fmt.Errorf("create CaveText account: %w", err))
		return
	}
	token, err := newToken()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// ponytail: expired token rows are never deleted, as with sessions.
	if _, err := a.db.ExecContext(r.Context(), `
		INSERT INTO cavetext_tokens (token_hash, learner_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		hashToken(token), learner.ID, caveTextTokenLifetime); err != nil {
		a.serverError(w, r, fmt.Errorf("store CaveText token: %w", err))
		return
	}
	http.Redirect(w, r, "https://"+id+".chromiumapp.org/#token="+token, http.StatusSeeOther)
}

type caveTextLearner struct {
	ID    int64  `json:"-"`
	Name  string `json:"name"`
	Place int64  `json:"place"`
	token []byte
}

// caveTextAPI authenticates a CaveText API call by its bearer token, never by cookie, and renews the token.
func (a *App) caveTextAPI(h func(http.ResponseWriter, *http.Request, *caveTextLearner)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeJSONError(w, http.StatusUnauthorized, "Sign in to CaveText again.")
			return
		}
		l := caveTextLearner{token: hashToken(token)}
		err := a.db.QueryRowContext(r.Context(), `
			UPDATE cavetext_tokens t SET expires_at = now() + $2::interval
			FROM learners l JOIN cavetext_accounts c ON c.learner_id = l.id
			WHERE t.token_hash = $1 AND t.expires_at > now() AND l.id = t.learner_id
			RETURNING l.id, l.name, c.place`, l.token, caveTextTokenLifetime).Scan(&l.ID, &l.Name, &l.Place)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusUnauthorized, "Sign in to CaveText again.")
			return
		}
		if err != nil {
			a.logError(r, fmt.Errorf("look up CaveText token: %w", err))
			writeJSONError(w, http.StatusInternalServerError, "CAVELMS had a problem. Try again later.")
			return
		}
		h(w, r, &l)
	}
}

func (a *App) caveTextMe(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	writeJSON(w, http.StatusOK, l)
}

func (a *App) caveTextSignOut(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM cavetext_tokens WHERE token_hash = $1`, l.token); err != nil {
		a.logError(r, fmt.Errorf("revoke CaveText token: %w", err))
		writeJSONError(w, http.StatusInternalServerError, "CAVELMS had a problem. Try again later.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError answers with a message the extension shows as is.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
