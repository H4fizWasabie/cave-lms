package app

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// A Teacher's own DSKPs (ADR 0029): a Teacher with Teachers Portal Pro uploads a DSKP the library lacks, the AI reads it as
// it reads the Admin's, and only that Teacher sees it, unchecked. It is never published to the library: every library
// query asks for owner_id IS NULL, and the Teacher's own is always asked for by its owner.

// maxTeacherDSKPUploads is how many DSKPs a Teacher may start uploading in a calendar month, Malaysia time.
const maxTeacherDSKPUploads = 5

var errUploadLimit = errors.New("DSKP upload limit reached")

// teacherDSKPUploadsLeft is how many uploads the account has left this calendar month, Malaysia time, and when the month
// ends and the count starts again.
func teacherDSKPUploadsLeft(ctx context.Context, q queryer, learnerID int64) (left int, resets time.Time, err error) {
	const monthKL = `date_trunc('month', now() AT TIME ZONE 'Asia/Kuala_Lumpur')`
	var used int
	err = q.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM teacher_dskp_uploads WHERE learner_id = $1 AND started_at >= `+monthKL+` AT TIME ZONE 'Asia/Kuala_Lumpur'),
		(`+monthKL+` + interval '1 month') AT TIME ZONE 'Asia/Kuala_Lumpur'`, learnerID).Scan(&used, &resets)
	return max(maxTeacherDSKPUploads-used, 0), resets.In(malaysia), err
}

// needsPro is what a Teacher who has no running Teachers Portal Pro is told uploading needs: when their Pro starts, if a
// period is queued after the trial, Malaysia time.
func needsPro(lang string, acc teacherAccess) string {
	if acc.ProStarts.IsZero() {
		return text.T(lang, "teachers.dskps.needs-pro")
	}
	return text.T(lang, "teachers.dskps.pro-starts", text.Date(lang, acc.ProStarts.In(malaysia)))
}

// teacherDSKPsPage is the Teachers Portal's list of DSKPs: the published ones that match the Teacher profile's subjects and
// years first, then the rest, and the Teacher's own uploads.
type teacherDSKPsPage struct {
	pageBase
	Section                 string
	Access                  teacherAccess
	Off                     bool
	CanUpload               bool
	NeedsPro                string // what a Teacher who cannot upload is told, in the page's language
	Years                   []string
	Subject, Year, Language string // the upload form as posted
	Mine, Others, Own       []dskp
	Left, Max               int       // uploads left this month, of Max
	LimitHit                bool      // this upload was refused for the limit
	Resets                  time.Time // when the month ends
}

// ResetsOn is the day the Teacher can upload again, when they have no uploads left.
func (p *teacherDSKPsPage) ResetsOn(lang string) string { return text.Date(lang, p.Resets) }

func (a *App) teacherDSKPs(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	a.renderTeacherDSKPs(w, r, http.StatusOK, acc, &teacherDSKPsPage{})
}

func (a *App) renderTeacherDSKPs(w http.ResponseWriter, r *http.Request, status int, acc teacherAccess, p *teacherDSKPsPage) {
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
	p.NeedsPro = needsPro(lang, acc)
	list, err := a.libraryDSKPs(r.Context(), dskpPublished)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Own, err = a.ownDSKPs(r.Context(), l.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Left, p.Resets, err = teacherDSKPUploadsLeft(r.Context(), a.db, l.ID); err != nil {
		a.serverError(w, r, fmt.Errorf("count DSKP uploads: %w", err))
		return
	}
	p.Max = maxTeacherDSKPUploads
	slices.SortFunc(list, func(x, y dskp) int {
		return cmp.Or(strings.Compare(strings.ToLower(x.Subject), strings.ToLower(y.Subject)), cmp.Compare(slices.Index(teacherYears, x.Year), slices.Index(teacherYears, y.Year)))
	})
	p.Section, p.Access, p.Off, p.Years = "dskps", acc, !a.writer.ReadsDSKP(), teacherYears
	p.CanUpload = acc.Pro() || acc.Admin()
	p.Mine, p.Others = nil, nil
	for _, d := range list {
		if acc.Profile.teaches(d) {
			p.Mine = append(p.Mine, d)
		} else {
			p.Others = append(p.Others, d)
		}
	}
	a.render(w, r, status, "teachers_dskps", p)
}

// ownDSKPs lists the DSKPs the account uploaded for themselves, newest first.
func (a *App) ownDSKPs(ctx context.Context, owner int64) ([]dskp, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+dskpColumns+` FROM dskps d WHERE d.owner_id = $1 ORDER BY d.id DESC`, owner)
	if err != nil {
		return nil, fmt.Errorf("list own DSKPs: %w", err)
	}
	defer rows.Close()
	var list []dskp
	for rows.Next() {
		d, err := scanDSKP(rows)
		if err != nil {
			return nil, fmt.Errorf("list own DSKPs: %w", err)
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// teacherUploadDSKP takes a DSKP a Teacher with Teachers Portal Pro uploads for themselves.
func (a *App) teacherUploadDSKP(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &teacherDSKPsPage{}
	fail := func(status int, key string) {
		p.Error = text.T(lang, key)
		a.renderTeacherDSKPs(w, r, status, acc, p)
	}
	switch {
	case !acc.Pro() && !acc.Admin():
		p.Error = needsPro(lang, acc)
		a.renderTeacherDSKPs(w, r, http.StatusForbidden, acc, p)
		return
	case !a.writer.ReadsDSKP():
		fail(http.StatusServiceUnavailable, "teachers.dskps.off")
		return
	}
	u, status, key, err := readDSKPUpload(w, r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Subject, p.Year, p.Language = u.Subject, u.Year, u.Language
	if key != "" {
		fail(status, key)
		return
	}
	id, err := a.saveOwnDSKP(r.Context(), l.ID, u)
	if errors.Is(err, errUploadLimit) {
		p.LimitHit = true // the page says so, and names the day
		a.renderTeacherDSKPs(w, r, http.StatusTooManyRequests, acc, p)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.startReadingDSKP(id, u)
	http.Redirect(w, r, portalPath+"/dskps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// teacherDSKP shows a published DSKP, or the Teacher's own upload in any state.
func (a *App) teacherDSKP(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	id, ok := dskpID(r)
	if !ok {
		a.notFound(w, r)
		return
	}
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	own := false
	d, groups, ok, err := a.libraryDSKP(r.Context(), id, dskpPublished)
	if err == nil && !ok {
		own = true
		d, groups, ok, err = a.dskpOf(r.Context(), sql.NullInt64{Int64: l.ID, Valid: true}, id, dskpReading, dskpFailed, dskpDraft)
	}
	switch {
	case err != nil:
		a.serverError(w, r, err)
	case !ok:
		a.notFound(w, r)
	default:
		a.render(w, r, http.StatusOK, "teachers_dskp", &dskpPage{Section: "dskps", Access: acc, Own: own, DSKP: d, Groups: groups})
	}
}

// saveOwnDSKP records an upload started by the account and saves its DSKP, unless the account has started its 5 this month.
// The account's row is locked meanwhile, so two uploads at once cannot both be the fifth.
func (a *App) saveOwnDSKP(ctx context.Context, owner int64, u dskpUpload) (id int64, err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, owner); err != nil {
		return 0, fmt.Errorf("lock account: %w", err)
	}
	left, _, err := teacherDSKPUploadsLeft(ctx, tx, owner)
	if err != nil {
		return 0, fmt.Errorf("count DSKP uploads: %w", err)
	}
	if left == 0 {
		return 0, errUploadLimit
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO teacher_dskp_uploads (learner_id) VALUES ($1)`, owner); err != nil {
		return 0, fmt.Errorf("record DSKP upload: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO dskps (subject, year, language, owner_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		u.Subject, u.Year, u.Language, owner).Scan(&id); err != nil {
		return 0, fmt.Errorf("save own DSKP: %w", err)
	}
	return id, tx.Commit()
}

// deleteOwnDSKP deletes a DSKP the Teacher uploaded, in whatever state it is. It gives no upload back: the month's count
// is of uploads started. A read still running for it finds it gone and changes nothing.
func (a *App) deleteOwnDSKP(w http.ResponseWriter, r *http.Request, acc teacherAccess) {
	id, ok := dskpID(r)
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		a.notFound(w, r)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM dskps WHERE id = $1 AND owner_id = $2`, id, l.ID)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("delete own DSKP: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		a.notFound(w, r)
		return
	}
	http.Redirect(w, r, portalPath+"/dskps", http.StatusSeeOther)
}

// teacherUploadRow is a subject and year Teachers uploaded a DSKP of for themselves, and how many Teachers did.
type teacherUploadRow struct {
	Subject, Year string
	Teachers      int
}

// teacherUploadsPage is the Admin's list of what Teachers uploaded for themselves, to see which DSKPs to add to the library.
type teacherUploadsPage struct {
	pageBase
	Rows []teacherUploadRow
}

// adminTeacherUploads lists, by subject (ignoring case) and year, the DSKPs Teachers uploaded for themselves and the AI read,
// most wanted first. It shows no Teacher and links to no DSKP: the Admin loads their own copy through the library.
func (a *App) adminTeacherUploads(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT (array_agg(d.subject ORDER BY d.id))[1], d.year, count(DISTINCT d.owner_id)
		FROM dskps d JOIN learners l ON l.id = d.owner_id
		WHERE d.state = 'draft' AND NOT l.is_admin
		GROUP BY lower(d.subject), d.year
		ORDER BY 3 DESC, lower(d.subject), d.year`)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("list Teacher uploads: %w", err))
		return
	}
	defer rows.Close()
	p := &teacherUploadsPage{}
	for rows.Next() {
		var row teacherUploadRow
		if err := rows.Scan(&row.Subject, &row.Year, &row.Teachers); err != nil {
			a.serverError(w, r, fmt.Errorf("list Teacher uploads: %w", err))
			return
		}
		p.Rows = append(p.Rows, row)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, fmt.Errorf("list Teacher uploads: %w", err))
		return
	}
	a.render(w, r, http.StatusOK, "admin_dskps_teachers", p)
}
