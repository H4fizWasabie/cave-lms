package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The Admin's hold on a DSKP after the AI read it: unpublish it, fix its standards, replace it with a new edition.

// unpublishDSKP takes a published DSKP back to a draft: Teachers stop seeing it, its standards stay. Unpublishing a draft
// changes nothing; a DSKP still being read or failed has nothing to unpublish.
func (a *App) unpublishDSKP(w http.ResponseWriter, r *http.Request) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE dskps SET state = 'draft' WHERE id = $1 AND owner_id IS NULL AND state = 'published'`, id)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("unpublish DSKP: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, _, found, err := a.libraryDSKP(r.Context(), id, dskpDraft); err != nil || !found {
			if err != nil {
				a.serverError(w, r, err)
			} else {
				a.notFound(w, r)
			}
			return
		}
	}
	http.Redirect(w, r, "/admin/dskps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// A new edition of a DSKP is uploaded from the DSKP's page, to /admin/dskps?replaces=<id>. It is a DSKP of its own, read and
// kept as a draft while Teachers keep the old one; publishing it takes the old one's place and removes it.

var errEditionPending = errors.New("a new edition is already being prepared")

// replacedDSKP is the id of the DSKP an upload is a new edition of, 0 for a plain upload; not ok when the request names
// one that is no id.
func replacedDSKP(r *http.Request) (id int64, ok bool) {
	if !r.URL.Query().Has("replaces") {
		return 0, true
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("replaces"), 10, 64)
	return id, err == nil && id > 0
}

// insertDSKP saves a DSKP that is about to be read. A new edition (replaces is the old DSKP's id) is only for a draft or
// published DSKP of the library that is no edition itself, and only one at a time: the last one failing makes way for the
// next.
func (a *App) insertDSKP(ctx context.Context, subject, year, language string, replaces int64) (id int64, err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var of sql.NullInt64
	if replaces != 0 {
		err = tx.QueryRowContext(ctx, `SELECT id FROM dskps WHERE id = $1 AND owner_id IS NULL AND state IN ('draft', 'published') AND replaces_id IS NULL FOR UPDATE`, replaces).Scan(&replaces)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNoStandard
		}
		if err != nil {
			return 0, err
		}
		of = sql.NullInt64{Int64: replaces, Valid: true}
		var pending bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM dskps WHERE replaces_id = $1 AND state <> 'failed')`, replaces).Scan(&pending); err != nil {
			return 0, err
		}
		if pending {
			return 0, errEditionPending
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM dskps WHERE replaces_id = $1`, replaces); err != nil { // the failed ones
			return 0, err
		}
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO dskps (subject, year, language, replaces_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		subject, year, language, of).Scan(&id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// publishDraft makes a draft of the library published; false when the DSKP is no draft. Publishing a new edition removes
// the DSKP it replaces, and any of its failed editions, in the same step.
func (a *App) publishDraft(ctx context.Context, id int64) (published bool, err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var replaced sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT replaces_id FROM dskps WHERE id = $1 AND owner_id IS NULL AND state = 'draft' FOR UPDATE`, id).Scan(&replaced)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dskps SET state = 'published', replaces_id = NULL WHERE id = $1`, id); err != nil {
		return false, err
	}
	if replaced.Valid {
		if _, err := tx.ExecContext(ctx, `DELETE FROM dskps WHERE id = $1 OR replaces_id = $1`, replaced.Int64); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// discardDSKP throws away a new edition, whatever state it is in, and leads back to the DSKP it was to replace, which
// stays as it was. Only a new edition can be discarded.
func (a *App) discardDSKP(w http.ResponseWriter, r *http.Request) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	var of int64
	err := a.db.QueryRowContext(r.Context(), `DELETE FROM dskps WHERE id = $1 AND owner_id IS NULL AND replaces_id IS NOT NULL RETURNING replaces_id`, id).Scan(&of)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		a.notFound(w, r)
	case err != nil:
		a.serverError(w, r, fmt.Errorf("discard DSKP: %w", err))
	default:
		http.Redirect(w, r, "/admin/dskps/"+strconv.FormatInt(of, 10), http.StatusSeeOther)
	}
}

// adminDSKPPage is the Admin's page of one DSKP: its standards, and the new edition being prepared of it or the DSKP it is a
// new edition of.
type adminDSKPPage struct {
	pageBase
	DSKP     dskp
	Groups   []dskpGroup
	Years    []string
	Off      bool  // DSKP reading is off
	Edition  *dskp // the latest new edition of this DSKP, whatever its state
	Replaces *dskp // the DSKP this is a new edition of
}

// CanReplace reports whether the page offers to upload a new edition: of a draft or published DSKP that is no edition
// itself, with none being prepared, while reading is on.
func (p *adminDSKPPage) CanReplace() bool {
	return !p.Off && p.Replaces == nil && (p.DSKP.Draft() || p.DSKP.Published()) && (p.Edition == nil || p.Edition.Failed())
}

func (a *App) renderAdminDSKP(w http.ResponseWriter, r *http.Request, status int, id int64, failure string) {
	d, groups, ok, err := a.libraryDSKP(r.Context(), id, dskpReading, dskpFailed, dskpDraft, dskpPublished)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		a.notFound(w, r)
		return
	}
	p := &adminDSKPPage{DSKP: d, Groups: groups, Years: teacherYears, Off: !a.writer.ReadsDSKP()}
	p.Error = failure
	for _, c := range []struct {
		into  **dskp
		where string
	}{
		{&p.Edition, `d.replaces_id = $1 ORDER BY d.id DESC`},
		{&p.Replaces, `d.id = (SELECT replaces_id FROM dskps WHERE id = $1)`},
	} {
		other, err := scanDSKP(a.db.QueryRowContext(r.Context(), `SELECT `+dskpColumns+` FROM dskps d WHERE `+c.where+` LIMIT 1`, id))
		if err == nil {
			*c.into = &other
		} else if !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return
		}
	}
	a.render(w, r, status, "admin_dskp", p)
}

// Limits of what the Admin types into a standard, in characters.
const (
	maxStandardName = 200  // a tema or bidang, a tajuk
	maxStandardCode = 40   // an SK's or SP's code
	maxStandardText = 2000 // an SK's or SP's text
)

var (
	errNoStandard  = errors.New("no such standard") // the edit names an SK or SP the DSKP does not have
	errBadStandard = errors.New("bad standard")     // the edit's fields are not acceptable
)

// editSP and editSK are the editor's view of the standards: the SKs flat in order, each knowing whether it starts a new
// tema or bidang and a new tajuk, with its SPs.
type editSP struct {
	ID         int64
	Code, Text string
}

type editSK struct {
	ID                 int64
	Grouping, Topic    string
	Code, Text         string
	NewGroup, NewTopic bool
	SPs                []editSP
}

type dskpEditPage struct {
	pageBase
	DSKP dskp
	SKs  []editSK
}

// editControls is what the editor's move and delete buttons of one block or SP need: where they post, the level of the
// block (none for an SP) and the language.
type editControls struct {
	Base, Level string
	Lang        string
}

// Controls are the buttons of sk's block of level: "sk", "topic" or "group".
func (p *dskpEditPage) Controls(sk editSK, level string) editControls {
	return editControls{Base: fmt.Sprintf("/admin/dskps/%d/sks/%d", p.DSKP.ID, sk.ID), Level: level, Lang: p.Lang}
}

// SPControls are the buttons of sp.
func (p *dskpEditPage) SPControls(sp editSP) editControls {
	return editControls{Base: fmt.Sprintf("/admin/dskps/%d/sps/%d", p.DSKP.ID, sp.ID), Lang: p.Lang}
}

func (a *App) editSKs(ctx context.Context, id int64) ([]editSK, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT k.id, k.grouping, k.topic, k.code, k.text, COALESCE(p.id, 0), COALESCE(p.code, ''), COALESCE(p.text, '')
		FROM dskp_sks k LEFT JOIN dskp_sps p ON p.sk_id = k.id
		WHERE k.dskp_id = $1 ORDER BY k.position, k.id, p.position, p.id`, id)
	if err != nil {
		return nil, fmt.Errorf("read DSKP standards: %w", err)
	}
	defer rows.Close()
	var sks []editSK
	for rows.Next() {
		var sk editSK
		var sp editSP
		if err := rows.Scan(&sk.ID, &sk.Grouping, &sk.Topic, &sk.Code, &sk.Text, &sp.ID, &sp.Code, &sp.Text); err != nil {
			return nil, fmt.Errorf("read DSKP standards: %w", err)
		}
		if n := len(sks); n == 0 || sks[n-1].ID != sk.ID {
			sk.NewGroup = n == 0 || sks[n-1].Grouping != sk.Grouping
			sk.NewTopic = sk.NewGroup || sks[n-1].Topic != sk.Topic
			sks = append(sks, sk)
		}
		if sp.ID != 0 {
			last := &sks[len(sks)-1]
			last.SPs = append(last.SPs, sp)
		}
	}
	return sks, rows.Err()
}

// adminDSKPEdit shows the editor of a draft or published DSKP's standards.
func (a *App) adminDSKPEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	a.renderDSKPEdit(w, r, http.StatusOK, id, "")
}

func (a *App) renderDSKPEdit(w http.ResponseWriter, r *http.Request, status int, id int64, failure string) {
	d, _, ok, err := a.libraryDSKP(r.Context(), id, dskpDraft, dskpPublished)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		a.notFound(w, r)
		return
	}
	sks, err := a.editSKs(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &dskpEditPage{DSKP: d, SKs: sks}
	p.Error = failure
	a.render(w, r, status, "admin_dskp_edit", p)
}

// editDSKP runs one edit of a draft or published DSKP's standards in a transaction that holds the DSKP, and leads back to
// its editor. An edit that names a standard the DSKP lacks is not found; one with unacceptable fields shows the editor
// again with what was wrong.
func (a *App) editDSKP(edit func(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := dskpID(r)
		if !ok {
			a.notFound(w, r)
			return
		}
		err := func() error {
			tx, err := a.db.BeginTx(r.Context(), nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			var held int64
			err = tx.QueryRowContext(r.Context(), `SELECT id FROM dskps WHERE id = $1 AND owner_id IS NULL AND state IN ('draft', 'published') FOR UPDATE`, id).Scan(&held)
			if errors.Is(err, sql.ErrNoRows) {
				return errNoStandard
			}
			if err != nil {
				return err
			}
			if err := edit(r.Context(), tx, id, r); err != nil {
				return err
			}
			return tx.Commit()
		}()
		switch {
		case err == nil:
			http.Redirect(w, r, "/admin/dskps/"+strconv.FormatInt(id, 10)+"/edit", http.StatusSeeOther)
		case errors.Is(err, errNoStandard):
			a.notFound(w, r)
		case errors.Is(err, errBadStandard):
			lang, err := a.language(r)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			a.renderDSKPEdit(w, r, http.StatusUnprocessableEntity, id, text.T(lang, "dskps.err.standard"))
		default:
			a.serverError(w, r, fmt.Errorf("edit DSKP: %w", err))
		}
	}
}

// pathID is the id in the request's path value name; 0, which no row has, when it is not a number.
func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

func field(r *http.Request, name string) string { return strings.TrimSpace(r.FormValue(name)) }

// codeAndText reads and checks the code and text of an SK or SP.
func codeAndText(r *http.Request) (code, txt string, err error) {
	code, txt = field(r, "code"), field(r, "text")
	if code == "" || txt == "" || utf8.RuneCountInString(code) > maxStandardCode || utf8.RuneCountInString(txt) > maxStandardText {
		return "", "", errBadStandard
	}
	return code, txt, nil
}

func changeSK(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	code, txt, err := codeAndText(r)
	if err != nil {
		return err
	}
	return changed(tx.ExecContext(ctx, `UPDATE dskp_sks SET code = $3, text = $4 WHERE id = $1 AND dskp_id = $2`, pathID(r, "sk"), id, code, txt))
}

func changeSP(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	code, txt, err := codeAndText(r)
	if err != nil {
		return err
	}
	return changed(tx.ExecContext(ctx, `UPDATE dskp_sps SET code = $3, text = $4
		WHERE id = $1 AND sk_id IN (SELECT id FROM dskp_sks WHERE dskp_id = $2)`, pathID(r, "sp"), id, code, txt))
}

func addSP(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	code, txt, err := codeAndText(r)
	if err != nil {
		return err
	}
	return changed(tx.ExecContext(ctx, `INSERT INTO dskp_sps (sk_id, position, code, text)
		SELECT k.id, COALESCE((SELECT max(position) + 1 FROM dskp_sps WHERE sk_id = k.id), 0), $3, $4
		FROM dskp_sks k WHERE k.id = $1 AND k.dskp_id = $2`, pathID(r, "sk"), id, code, txt))
}

// skRow is an SK as the ordering sees it: where it stands and what it is grouped under.
type skRow struct {
	ID              int64
	Grouping, Topic string
}

func skRows(ctx context.Context, tx *sql.Tx, id int64) ([]skRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, grouping, topic FROM dskp_sks WHERE dskp_id = $1 ORDER BY position, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []skRow
	for rows.Next() {
		var row skRow
		if err := rows.Scan(&row.ID, &row.Grouping, &row.Topic); err != nil {
			return nil, err
		}
		list = append(list, row)
	}
	return list, rows.Err()
}

// setSKOrder makes the SKs stand in the order of rows.
func setSKOrder(ctx context.Context, tx *sql.Tx, rows []skRow) error {
	for i, row := range rows {
		if _, err := tx.ExecContext(ctx, `UPDATE dskp_sks SET position = $2 WHERE id = $1`, row.ID, i); err != nil {
			return err
		}
	}
	return nil
}

// addSK adds an SK at the end of the SKs with its grouping and tajuk when there are some, else at the end of those with its
// grouping, else at the end of the DSKP: a new grouping is an SK under a grouping the DSKP does not have yet.
func addSK(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	code, txt, err := codeAndText(r)
	if err != nil {
		return err
	}
	grouping, topic := field(r, "grouping"), field(r, "topic")
	if utf8.RuneCountInString(grouping) > maxStandardName || utf8.RuneCountInString(topic) > maxStandardName {
		return errBadStandard
	}
	rows, err := skRows(ctx, tx, id)
	if err != nil {
		return err
	}
	at, inGrouping := len(rows), -1
	for i, row := range rows {
		if row.Grouping == grouping {
			inGrouping = i + 1
		}
	}
	if inGrouping >= 0 {
		at = inGrouping
		for i, row := range rows {
			if row.Grouping == grouping && row.Topic == topic {
				at = i + 1
			}
		}
	}
	var added skRow
	added.Grouping, added.Topic = grouping, topic
	if err := tx.QueryRowContext(ctx, `INSERT INTO dskp_sks (dskp_id, position, grouping, topic, code, text) VALUES ($1, 0, $2, $3, $4, $5) RETURNING id`,
		id, grouping, topic, code, txt).Scan(&added.ID); err != nil {
		return err
	}
	return setSKOrder(ctx, tx, slices.Insert(rows, at, added))
}

// blockOf finds the SK the request names among rows and the block it stands for: the SK alone, its tajuk or its tema or
// bidang, as the request's level says, with where it starts and ends in rows. An SK or a level that is not there is
// errNoStandard.
func blockOf(rows []skRow, r *http.Request) (level string, start, end int, err error) {
	i := slices.IndexFunc(rows, func(row skRow) bool { return row.ID == pathID(r, "sk") })
	level = r.FormValue("level")
	if i < 0 || !slices.Contains([]string{"", "sk", "topic", "group"}, level) {
		return "", 0, 0, errNoStandard
	}
	start, end = blockAt(rows, i, level)
	return level, start, end, nil
}

// blockAt is where the block of level around rows[i] starts and ends: the SK alone, or the SKs of the same tajuk or the
// same tema or bidang that stand together.
func blockAt(rows []skRow, i int, level string) (start, end int) {
	same := func(a, b skRow) bool {
		switch level {
		case "group":
			return a.Grouping == b.Grouping
		case "topic":
			return a.Grouping == b.Grouping && a.Topic == b.Topic
		}
		return false
	}
	start, end = i, i+1
	for start > 0 && same(rows[start-1], rows[i]) {
		start--
	}
	for end < len(rows) && same(rows[end], rows[i]) {
		end++
	}
	return start, end
}

// sameHolder reports whether two blocks of level are held by the same one: SKs by one tajuk, tajuk by one tema, temas by
// the DSKP.
func sameHolder(level string, a, b skRow) bool {
	switch level {
	case "group":
		return true
	case "topic":
		return a.Grouping == b.Grouping
	}
	return a.Grouping == b.Grouping && a.Topic == b.Topic
}

// moveSK moves the block the request names above or below the next block of its level, as long as the same one holds
// both: an SK moves among the SKs of its tajuk, a tajuk among those of its tema, a tema among the temas. At the end of its
// holder it stays.
func moveSK(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	rows, err := skRows(ctx, tx, id)
	if err != nil {
		return err
	}
	level, start, end, err := blockOf(rows, r)
	if err != nil {
		return err
	}
	var upper, lower [2]int // the two blocks to swap, each as [start, end) of rows
	switch r.FormValue("dir") {
	case "up":
		if start == 0 {
			return nil
		}
		ps, pe := blockAt(rows, start-1, level)
		upper, lower = [2]int{ps, pe}, [2]int{start, end}
	case "down":
		if end == len(rows) {
			return nil
		}
		ns, ne := blockAt(rows, end, level)
		upper, lower = [2]int{start, end}, [2]int{ns, ne}
	default:
		return errNoStandard
	}
	if !sameHolder(level, rows[upper[0]], rows[lower[0]]) {
		return nil
	}
	return setSKOrder(ctx, tx, slices.Concat(rows[:upper[0]], rows[lower[0]:lower[1]], rows[upper[0]:upper[1]], rows[lower[1]:]))
}

// moveSP moves an SP above or below its neighbour in its SK; at the end of its SK it stays.
func moveSP(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	rows, err := tx.QueryContext(ctx, `SELECT p.id FROM dskp_sps p
		WHERE p.sk_id = (SELECT sp.sk_id FROM dskp_sps sp JOIN dskp_sks k ON k.id = sp.sk_id WHERE sp.id = $1 AND k.dskp_id = $2)
		ORDER BY p.position, p.id`, pathID(r, "sp"), id)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var spID int64
		if err := rows.Scan(&spID); err != nil {
			return err
		}
		ids = append(ids, spID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	i, dir := slices.Index(ids, pathID(r, "sp")), r.FormValue("dir")
	if i < 0 || (dir != "up" && dir != "down") {
		return errNoStandard
	}
	to := i + 1
	if dir == "up" {
		to = i - 1
	}
	if to < 0 || to >= len(ids) {
		return nil
	}
	ids[i], ids[to] = ids[to], ids[i]
	for pos, spID := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE dskp_sps SET position = $2 WHERE id = $1`, spID, pos); err != nil {
			return err
		}
	}
	return nil
}

func deleteSK(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	rows, err := skRows(ctx, tx, id)
	if err != nil {
		return err
	}
	_, start, end, err := blockOf(rows, r)
	if err != nil {
		return err
	}
	var gone []int64
	for _, row := range rows[start:end] {
		gone = append(gone, row.ID)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM dskp_sks WHERE dskp_id = $1 AND id = ANY($2)`, id, gone)
	return err
}

// renameSK gives the tema or bidang, or the tajuk, of the SK the request names a new name: to all the SKs that stand
// under it.
func renameSK(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	rows, err := skRows(ctx, tx, id)
	if err != nil {
		return err
	}
	level, start, end, err := blockOf(rows, r)
	if err != nil || (level != "group" && level != "topic") {
		return errNoStandard
	}
	name := field(r, "name")
	if utf8.RuneCountInString(name) > maxStandardName {
		return errBadStandard
	}
	column := map[string]string{"group": "grouping", "topic": "topic"}[level]
	var renamed []int64
	for _, row := range rows[start:end] {
		renamed = append(renamed, row.ID)
	}
	_, err = tx.ExecContext(ctx, `UPDATE dskp_sks SET `+column+` = $3 WHERE dskp_id = $1 AND id = ANY($2)`, id, renamed, name)
	return err
}

func deleteSP(ctx context.Context, tx *sql.Tx, id int64, r *http.Request) error {
	return changed(tx.ExecContext(ctx, `DELETE FROM dskp_sps WHERE id = $1 AND sk_id IN (SELECT id FROM dskp_sks WHERE dskp_id = $2)`, pathID(r, "sp"), id))
}

// changed is errNoStandard when the statement changed no row.
func changed(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNoStandard
	}
	return nil
}
