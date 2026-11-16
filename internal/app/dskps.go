package app

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The DSKP library (ADR 0029): the Admin uploads a DSKP's PDF, the AI reads its standards in the background, the Admin
// publishes it, and Teachers browse the published ones. The PDF lives in memory only until the read ends: nothing of
// the file is stored.
const (
	maxDSKPBytes   = 30 << 20 // about 30 MB
	maxDSKPSubject = 100
)

// DSKP states: a DSKP is being read, the read failed, or it is read and a draft, then published.
const (
	dskpReading   = "reading"
	dskpFailed    = "failed"
	dskpDraft     = "draft"
	dskpPublished = "published"
)

// dskp is one DSKP of the library.
type dskp struct {
	ID                      int64
	Subject, Year, Language string
	State, Failure          string
	SKs, SPs                int // how many standards were read
}

func (d dskp) Reading() bool   { return d.State == dskpReading }
func (d dskp) Failed() bool    { return d.State == dskpFailed }
func (d dskp) Draft() bool     { return d.State == dskpDraft }
func (d dskp) Published() bool { return d.State == dskpPublished }

// dskpSK is a Standard Kandungan with its Standard Pembelajaran; dskpGroup and dskpTopic are the grouping (tema or
// bidang) and tajuk the DSKP puts it under, in the DSKP's order.
type dskpSK struct {
	Code, Text string
	SPs        []aiwriter.SP
}

type dskpTopic struct {
	Title string
	SKs   []dskpSK
}

type dskpGroup struct {
	Name   string
	Topics []dskpTopic
}

const dskpColumns = `d.id, d.subject, d.year, d.language, d.state, d.failure,
	(SELECT count(*) FROM dskp_sks WHERE dskp_id = d.id),
	(SELECT count(*) FROM dskp_sps p JOIN dskp_sks k ON k.id = p.sk_id WHERE k.dskp_id = d.id)`

func scanDSKP(row interface{ Scan(...any) error }) (dskp, error) {
	var d dskp
	err := row.Scan(&d.ID, &d.Subject, &d.Year, &d.Language, &d.State, &d.Failure, &d.SKs, &d.SPs)
	return d, err
}

// libraryDSKPs lists the library's DSKPs in the given states, newest first.
func (a *App) libraryDSKPs(ctx context.Context, states ...string) ([]dskp, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+dskpColumns+` FROM dskps d
		WHERE d.owner_id IS NULL AND d.state = ANY($1) ORDER BY d.id DESC`, states)
	if err != nil {
		return nil, fmt.Errorf("list DSKPs: %w", err)
	}
	defer rows.Close()
	var list []dskp
	for rows.Next() {
		d, err := scanDSKP(rows)
		if err != nil {
			return nil, fmt.Errorf("list DSKPs: %w", err)
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// libraryDSKP reads one DSKP of the library with its standards grouped as the DSKP groups them; ok is false when there
// is none, or when it is not in one of states.
func (a *App) libraryDSKP(ctx context.Context, id int64, states ...string) (d dskp, groups []dskpGroup, ok bool, err error) {
	d, err = scanDSKP(a.db.QueryRowContext(ctx, `SELECT `+dskpColumns+` FROM dskps d
		WHERE d.id = $1 AND d.owner_id IS NULL AND d.state = ANY($2)`, id, states))
	if errors.Is(err, sql.ErrNoRows) {
		return d, nil, false, nil
	}
	if err != nil {
		return d, nil, false, fmt.Errorf("read DSKP: %w", err)
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT k.id, k.grouping, k.topic, k.code, k.text, COALESCE(p.code, ''), COALESCE(p.text, '')
		FROM dskp_sks k LEFT JOIN dskp_sps p ON p.sk_id = k.id
		WHERE k.dskp_id = $1 ORDER BY k.position, p.position`, id)
	if err != nil {
		return d, nil, false, fmt.Errorf("read DSKP standards: %w", err)
	}
	defer rows.Close()
	var lastSK int64
	for rows.Next() {
		var skID int64
		var grouping, topic string
		var sk dskpSK
		var sp aiwriter.SP
		if err := rows.Scan(&skID, &grouping, &topic, &sk.Code, &sk.Text, &sp.Code, &sp.Text); err != nil {
			return d, nil, false, fmt.Errorf("read DSKP standards: %w", err)
		}
		if skID != lastSK { // the next SK: under the same grouping and tajuk as the last, or a new one
			lastSK = skID
			if len(groups) == 0 || groups[len(groups)-1].Name != grouping {
				groups = append(groups, dskpGroup{Name: grouping})
			}
			g := &groups[len(groups)-1]
			if len(g.Topics) == 0 || g.Topics[len(g.Topics)-1].Title != topic {
				g.Topics = append(g.Topics, dskpTopic{Title: topic})
			}
			t := &g.Topics[len(g.Topics)-1]
			t.SKs = append(t.SKs, sk)
		}
		if sp.Code != "" {
			g := &groups[len(groups)-1]
			t := &g.Topics[len(g.Topics)-1]
			sk := &t.SKs[len(t.SKs)-1]
			sk.SPs = append(sk.SPs, sp)
		}
	}
	return d, groups, true, rows.Err()
}

// dskpsPage is the Admin's DSKP library: its list, and the upload form unless reading is off.
type dskpsPage struct {
	pageBase
	Off                     bool // DSKP reading is off: its model is not set
	DSKPs                   []dskp
	Years                   []string
	Subject, Year, Language string // the upload form as posted
}

func (a *App) adminDSKPs(w http.ResponseWriter, r *http.Request) {
	a.renderDSKPs(w, r, http.StatusOK, &dskpsPage{})
}

func (a *App) renderDSKPs(w http.ResponseWriter, r *http.Request, status int, p *dskpsPage) {
	list, err := a.libraryDSKPs(r.Context(), dskpReading, dskpFailed, dskpDraft, dskpPublished)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Off, p.DSKPs, p.Years = !a.writer.ReadsDSKP(), list, teacherYears
	a.render(w, r, status, "admin_dskps", p)
}

// uploadDSKP takes a DSKP's PDF, subject, year and language, and starts the read in the background. The PDF is held in
// memory only: nothing of it reaches the disk or the database.
func (a *App) uploadDSKP(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &dskpsPage{}
	fail := func(status int, key string) {
		p.Error = text.T(lang, key)
		a.renderDSKPs(w, r, status, p)
	}
	if !a.writer.ReadsDSKP() {
		fail(http.StatusServiceUnavailable, "dskps.off")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDSKPBytes+64<<10)
	if err := r.ParseMultipartForm(maxDSKPBytes + 1<<20); err != nil && !errors.Is(err, http.ErrNotMultipart) { // held in memory
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(http.StatusRequestEntityTooLarge, "dskps.err.too-large")
			return
		}
		fail(http.StatusBadRequest, "dskps.err.unread")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	p.Subject, p.Year, p.Language = strings.Join(strings.Fields(r.FormValue("subject")), " "), r.FormValue("year"), r.FormValue("language")
	switch {
	case p.Subject == "" || utf8.RuneCountInString(p.Subject) > maxDSKPSubject:
		fail(http.StatusUnprocessableEntity, "dskps.err.subject")
		return
	case !slices.Contains(teacherYears, p.Year):
		fail(http.StatusUnprocessableEntity, "dskps.err.year")
		return
	case p.Language != "ms" && p.Language != "en":
		fail(http.StatusUnprocessableEntity, "dskps.err.language")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(http.StatusUnprocessableEntity, "dskps.err.pdf")
		return
	}
	defer file.Close()
	if header.Size > maxDSKPBytes {
		fail(http.StatusRequestEntityTooLarge, "dskps.err.too-large")
		return
	}
	pdf, err := io.ReadAll(file)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		fail(http.StatusUnprocessableEntity, "dskps.err.pdf")
		return
	}
	var id int64
	if err := a.db.QueryRowContext(r.Context(), `INSERT INTO dskps (subject, year, language) VALUES ($1, $2, $3) RETURNING id`,
		p.Subject, p.Year, p.Language).Scan(&id); err != nil {
		a.serverError(w, r, fmt.Errorf("save DSKP: %w", err))
		return
	}
	a.reading.Add(1)
	go a.readDSKP(id, aiwriter.DSKPRequest{PDF: pdf, Language: p.Language})
	http.Redirect(w, r, "/admin/dskps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// readDSKP reads the standards from the PDF and saves them, which makes the DSKP a draft, or records why it failed. It
// is the one holder of the PDF once the upload answers, and lets go of it as the read ends. A read a restart or Close
// cut short is a failure too; saving after the DSKP was failed already, by a restart's sweep, changes nothing.
func (a *App) readDSKP(id int64, req aiwriter.DSKPRequest) {
	defer a.reading.Done()
	a.survive("DSKP reading", func() {
		sks, err := aiwriter.ReadDSKP(a.readCtx, a.writer, req)
		req.PDF = nil // the PDF is not kept
		if err == nil {
			err = a.saveDSKPStandards(id, sks)
		}
		if err != nil {
			failure := dskpFailure(err)
			if failure == "unavailable" {
				a.logError(nil, fmt.Errorf("read DSKP %d: %w", id, err))
			}
			if _, err := a.db.ExecContext(context.Background(), `UPDATE dskps SET state = 'failed', failure = $2 WHERE id = $1 AND state = 'reading'`, id, failure); err != nil {
				a.logError(nil, fmt.Errorf("record DSKP %d failed: %w", id, err))
			}
		}
	})
}

// dskpFailure is the reason a read failed, as dskps.failure.* names it.
func dskpFailure(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "restart"
	case errors.Is(err, aiwriter.ErrNotConfigured):
		return "not-configured"
	case errors.Is(err, aiwriter.ErrOverLimit):
		return "over-limit"
	case errors.Is(err, aiwriter.ErrBadShape):
		return "unreadable"
	}
	return "unavailable"
}

// saveDSKPStandards makes the DSKP a draft with the standards that were read, unless it is no longer being read.
func (a *App) saveDSKPStandards(id int64, sks []aiwriter.SK) error {
	ctx := context.Background()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE dskps SET state = 'draft' WHERE id = $1 AND state = 'reading'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // failed by a restart's sweep meanwhile
	}
	for i, sk := range sks {
		var skID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO dskp_sks (dskp_id, position, grouping, topic, code, text) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			id, i, strings.TrimSpace(sk.Grouping), strings.TrimSpace(sk.Topic), strings.TrimSpace(sk.Code), strings.TrimSpace(sk.Text)).Scan(&skID); err != nil {
			return err
		}
		for j, sp := range sk.SPs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO dskp_sps (sk_id, position, code, text) VALUES ($1, $2, $3, $4)`,
				skID, j, strings.TrimSpace(sp.Code), strings.TrimSpace(sp.Text)); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// failInterruptedDSKPReads marks every DSKP still being read as failed: a restart ended the read that was holding its PDF,
// and the Admin uploads it again.
func (a *App) failInterruptedDSKPReads(ctx context.Context) error {
	if _, err := a.db.ExecContext(ctx, `UPDATE dskps SET state = 'failed', failure = 'restart' WHERE state = 'reading'`); err != nil {
		return fmt.Errorf("fail interrupted DSKP reads: %w", err)
	}
	return nil
}

// dskpPage is one DSKP with its standards, for the Admin and for Teachers.
type dskpPage struct {
	pageBase
	Section string
	Access  teacherAccess
	DSKP    dskp
	Groups  []dskpGroup
}

func dskpID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func (a *App) adminDSKP(w http.ResponseWriter, r *http.Request) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	d, groups, ok, err := a.libraryDSKP(r.Context(), id, dskpReading, dskpFailed, dskpDraft, dskpPublished)
	switch {
	case err != nil:
		a.serverError(w, r, err)
	case !ok:
		a.notFound(w, r)
	default:
		a.render(w, r, http.StatusOK, "admin_dskp", &dskpPage{DSKP: d, Groups: groups})
	}
}

// publishDSKP makes a draft visible to Teachers. Publishing a published DSKP changes nothing; a DSKP still being read or
// failed cannot be published.
func (a *App) publishDSKP(w http.ResponseWriter, r *http.Request) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE dskps SET state = 'published' WHERE id = $1 AND owner_id IS NULL AND state = 'draft'`, id)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("publish DSKP: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, _, found, err := a.libraryDSKP(r.Context(), id, dskpPublished); err != nil || !found {
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

// teacherPage runs h for an Account holder the portal lets in: the Admin or a Teacher on the trial. Without a Teacher
// profile it asks for one; once the trial has ended it leads to the portal's home.
func (a *App) teacherPage(h func(http.ResponseWriter, *http.Request, teacherAccess)) http.HandlerFunc {
	return a.requireLearner(func(w http.ResponseWriter, r *http.Request) {
		l, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		acc, err := a.teacherAccessOf(r.Context(), l)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		switch acc.State {
		case teacherNone:
			http.Redirect(w, r, portalPath+"/profile", http.StatusSeeOther)
		case teacherLapsed:
			http.Redirect(w, r, portalPath, http.StatusSeeOther)
		default:
			h(w, r, acc)
		}
	})
}

// teacherDSKPsPage is the Teachers Portal's list of published DSKPs: the ones that match the Teacher profile's subjects and
// years first, then the rest.
type teacherDSKPsPage struct {
	pageBase
	Section      string
	Access       teacherAccess
	Off          bool
	Mine, Others []dskp
}

func (a *App) teacherDSKPs(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	list, err := a.libraryDSKPs(r.Context(), dskpPublished)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	slices.SortFunc(list, func(x, y dskp) int {
		return cmp.Or(strings.Compare(strings.ToLower(x.Subject), strings.ToLower(y.Subject)), cmp.Compare(slices.Index(teacherYears, x.Year), slices.Index(teacherYears, y.Year)))
	})
	p := &teacherDSKPsPage{Section: "dskps", Access: acc, Off: !a.writer.ReadsDSKP()}
	for _, d := range list {
		if acc.Profile.teaches(d) {
			p.Mine = append(p.Mine, d)
		} else {
			p.Others = append(p.Others, d)
		}
	}
	a.render(w, r, http.StatusOK, "teachers_dskps", p)
}

// teaches reports whether the DSKP is one of the Teacher's: its subject is one of the profile's, ignoring case, and its
// year one of the profile's years. The Admin has no profile, so no DSKP is theirs.
func (p *teacherProfile) teaches(d dskp) bool {
	if p == nil || !slices.Contains(p.Years, d.Year) {
		return false
	}
	for _, s := range strings.Split(p.Subjects, ", ") {
		if strings.EqualFold(s, d.Subject) {
			return true
		}
	}
	return false
}

func (a *App) teacherDSKP(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	d, groups, ok, err := a.libraryDSKP(r.Context(), id, dskpPublished)
	switch {
	case err != nil:
		a.serverError(w, r, err)
	case !ok:
		a.notFound(w, r)
	default:
		a.render(w, r, http.StatusOK, "teachers_dskp", &dskpPage{Section: "dskps", Access: acc, DSKP: d, Groups: groups})
	}
}
