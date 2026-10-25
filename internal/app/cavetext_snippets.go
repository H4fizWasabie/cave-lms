package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// caveTextSnippet is a Snippet as the CaveText API sends and takes it.
type caveTextSnippet struct {
	ID               int64  `json:"id"`
	Folder           string `json:"folder"`
	Name             string `json:"name"`
	Shortcut         string `json:"shortcut"`
	Body             string `json:"body"`
	WithMasterPrompt bool   `json:"withMasterPrompt"`
}

func (a *App) caveTextSnippets(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT id, folder, name, shortcut, body, with_master_prompt FROM cavetext_snippets
		WHERE learner_id = $1 ORDER BY id`, l.ID)
	if err != nil {
		a.logError(r, fmt.Errorf("list Snippets: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
		return
	}
	defer rows.Close()
	list := []caveTextSnippet{}
	for rows.Next() {
		var s caveTextSnippet
		if err := rows.Scan(&s.ID, &s.Folder, &s.Name, &s.Shortcut, &s.Body, &s.WithMasterPrompt); err != nil {
			a.logError(r, fmt.Errorf("list Snippets: %w", err))
			writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
			return
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, fmt.Errorf("list Snippets: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snippets": list})
}

// snippetIn is a Snippet as CaveText sends it; "withMasterPrompt" is on unless sent as false.
type snippetIn struct {
	Folder, Name, Shortcut, Body string
	WithMasterPrompt             *bool
}

func (in snippetIn) snippet() caveTextSnippet {
	return caveTextSnippet{Folder: in.Folder, Name: in.Name, Shortcut: in.Shortcut, Body: in.Body, WithMasterPrompt: in.WithMasterPrompt == nil || *in.WithMasterPrompt}
}

// readSnippet reads a Snippet from the request body.
func readSnippet(w http.ResponseWriter, r *http.Request) (caveTextSnippet, bool) {
	var in snippetIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "CaveText sent a Snippet CAVELMS can't read.")
		return caveTextSnippet{}, false
	}
	return in.snippet(), true
}

// lockSnippets begins a transaction holding the Learner's account row, so two browsers saving at once can't both
// slip a clash past the checks.
func (a *App) lockSnippets(r *http.Request, l *caveTextLearner) (*sql.Tx, error) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM cavetext_accounts WHERE learner_id = $1 FOR UPDATE`, l.ID); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// otherSnippets lists the Learner's Snippets but the one with id except, for the clash check.
func otherSnippets(r *http.Request, tx *sql.Tx, l *caveTextLearner, except int64) ([]caveTextSnippet, error) {
	rows, err := tx.QueryContext(r.Context(), `SELECT name, shortcut FROM cavetext_snippets WHERE learner_id = $1 AND id <> $2`, l.ID, except)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var others []caveTextSnippet
	for rows.Next() {
		var o caveTextSnippet
		if err := rows.Scan(&o.Name, &o.Shortcut); err != nil {
			return nil, err
		}
		others = append(others, o)
	}
	return others, rows.Err()
}

// A Snippet's name, folder and shortcut have up to maxSnippetField characters each, its body up to maxSnippetBody.
const maxSnippetField, maxSnippetBody = 100, 20000

// snippetProblem says, in lang, why a Snippet can't be saved beside the others, or "".
func snippetProblem(lang string, s caveTextSnippet, others []caveTextSnippet) string {
	if strings.TrimSpace(s.Name) == "" {
		return text.T(lang, "cavetext.api.name-needed")
	}
	if utf8.RuneCountInString(s.Name) > maxSnippetField || utf8.RuneCountInString(s.Folder) > maxSnippetField ||
		utf8.RuneCountInString(s.Shortcut) > maxSnippetField || utf8.RuneCountInString(s.Body) > maxSnippetBody {
		return text.T(lang, "cavetext.api.too-long")
	}
	return shortcutProblem(lang, s.Shortcut, others)
}

func (a *App) createCaveTextSnippet(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	s, ok := readSnippet(w, r)
	if !ok {
		return
	}
	fail := func(err error) {
		a.logError(r, fmt.Errorf("create Snippet: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
	}
	tx, err := a.lockSnippets(r, l)
	if err != nil {
		fail(err)
		return
	}
	defer tx.Rollback()
	others, err := otherSnippets(r, tx, l, 0)
	if err != nil {
		fail(err)
		return
	}
	reserved, err := reservedShortcuts(r.Context(), tx)
	if err != nil {
		fail(err)
		return
	}
	if problem := snippetProblem(l.Lang, s, clashList(reserved, others, 0)); problem != "" {
		writeJSONError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	c, err := learnerSnippetCap(r.Context(), tx, l.ID)
	if err != nil {
		fail(err)
		return
	}
	if c.Full(len(others)) {
		writeJSONError(w, http.StatusForbidden, overCapMessage(l.Lang, len(others), c))
		return
	}
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO cavetext_snippets (learner_id, folder, name, shortcut, body, with_master_prompt)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		l.ID, s.Folder, s.Name, s.Shortcut, s.Body, s.WithMasterPrompt).Scan(&s.ID); err != nil {
		fail(err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail(err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// Last write wins: an update replaces the Snippet whatever another browser wrote in between.
func (a *App) updateCaveTextSnippet(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeJSONError(w, http.StatusNotFound, text.T(l.Lang, "cavetext.api.snippet-gone"))
		return
	}
	s, ok := readSnippet(w, r)
	if !ok {
		return
	}
	fail := func(err error) {
		a.logError(r, fmt.Errorf("update Snippet: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
	}
	tx, err := a.lockSnippets(r, l)
	if err != nil {
		fail(err)
		return
	}
	defer tx.Rollback()
	others, err := otherSnippets(r, tx, l, id)
	if err != nil {
		fail(err)
		return
	}
	reserved, err := reservedShortcuts(r.Context(), tx)
	if err != nil {
		fail(err)
		return
	}
	if problem := snippetProblem(l.Lang, s, clashList(reserved, others, 0)); problem != "" {
		writeJSONError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	err = tx.QueryRowContext(r.Context(), `
		UPDATE cavetext_snippets SET folder = $3, name = $4, shortcut = $5, body = $6, with_master_prompt = $7, updated_at = now()
		WHERE id = $1 AND learner_id = $2 RETURNING id`,
		id, l.ID, s.Folder, s.Name, s.Shortcut, s.Body, s.WithMasterPrompt).Scan(&s.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, text.T(l.Lang, "cavetext.api.snippet-gone"))
		return
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// replaceCaveTextSnippets replaces the Learner's whole Snippet set (an import) in one transaction. A set over the cap
// is cut to the cap first, and the answer says how many were skipped. Every kept Snippet is then checked against the
// master prompts' shortcuts and the kept ones before it; one refused and nothing is written.
func (a *App) replaceCaveTextSnippets(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	var in struct{ Snippets []snippetIn }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "CaveText sent Snippets CAVELMS can't read.")
		return
	}
	fail := func(err error) {
		a.logError(r, fmt.Errorf("replace Snippets: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
	}
	tx, err := a.lockSnippets(r, l)
	if err != nil {
		fail(err)
		return
	}
	defer tx.Rollback()
	c, err := learnerSnippetCap(r.Context(), tx, l.ID)
	if err != nil {
		fail(err)
		return
	}
	reserved, err := reservedShortcuts(r.Context(), tx)
	if err != nil {
		fail(err)
		return
	}
	skipped := len(in.Snippets) - c.Keep(len(in.Snippets))
	list := make([]caveTextSnippet, len(in.Snippets)-skipped)
	before := clashList(reserved, nil, len(list)) // grows by each kept Snippet once it passes
	for i := range list {
		if r.Context().Err() != nil {
			return // the request is gone; stop the clash check, and the deferred Rollback frees the lock
		}
		list[i] = in.Snippets[i].snippet()
		if problem := snippetProblem(l.Lang, list[i], before); problem != "" {
			writeJSONError(w, http.StatusUnprocessableEntity, problem)
			return
		}
		before = append(before, list[i])
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM cavetext_snippets WHERE learner_id = $1`, l.ID); err != nil {
		fail(err)
		return
	}
	for i := range list {
		s := &list[i]
		if err := tx.QueryRowContext(r.Context(), `
			INSERT INTO cavetext_snippets (learner_id, folder, name, shortcut, body, with_master_prompt)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			l.ID, s.Folder, s.Name, s.Shortcut, s.Body, s.WithMasterPrompt).Scan(&s.ID); err != nil {
			fail(err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(err)
		return
	}
	answer := map[string]any{"snippets": list, "skipped": skipped}
	if skipped > 0 {
		answer["message"] = skippedMessage(l.Lang, len(list), skipped, c)
	}
	writeJSON(w, http.StatusOK, answer)
}

// shortcutProblem says why a shortcut can't be saved beside the others, or "". These are the rules the extension's
// dashboard uses: a shortcut has no whitespace and, ignoring case, is neither another's nor a prefix of one or the
// other way round, since the shorter would always fire first.
func shortcutProblem(lang, shortcut string, others []caveTextSnippet) string {
	if shortcut == "" {
		return text.T(lang, "cavetext.api.shortcut-needed")
	}
	if strings.IndexFunc(shortcut, unicode.IsSpace) >= 0 {
		return text.T(lang, "cavetext.api.shortcut-spaces")
	}
	s := strings.ToLower(shortcut)
	for _, o := range others {
		t := strings.ToLower(o.Shortcut)
		switch {
		case t == s:
			return text.T(lang, "cavetext.api.shortcut-taken", o.Name, o.Shortcut)
		case strings.HasPrefix(t, s):
			return text.T(lang, "cavetext.api.shortcut-shorter", shortcut, o.Shortcut, o.Name)
		case strings.HasPrefix(s, t):
			return text.T(lang, "cavetext.api.shortcut-longer", o.Shortcut, o.Name, shortcut)
		}
	}
	return ""
}

func (a *App) deleteCaveTextSnippet(w http.ResponseWriter, r *http.Request, l *caveTextLearner) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, text.T(l.Lang, "cavetext.api.snippet-gone"))
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM cavetext_snippets WHERE id = $1 AND learner_id = $2`, id, l.ID)
	if err != nil {
		a.logError(r, fmt.Errorf("delete Snippet: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.logError(r, fmt.Errorf("delete Snippet: %w", err))
		writeJSONError(w, http.StatusInternalServerError, text.T(l.Lang, "cavetext.api.problem"))
		return
	}
	if n == 0 {
		writeJSONError(w, http.StatusNotFound, text.T(l.Lang, "cavetext.api.snippet-gone"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
