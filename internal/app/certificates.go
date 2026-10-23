package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/go-pdf/fpdf"
)

// maxCertificateImage caps the form that carries the logo and signature.
const maxCertificateImage = 5 << 20

// certificate is one issued Certificate. LearnerName, LearnerIC, CourseTitle and Days are copies taken at issue time.
type certificate struct {
	ID          string
	Year        int // Year and Seq make the Certificate No.
	Seq         int
	LearnerID   int64
	LearnerName string
	LearnerIC   string // stored form, "" on Certificates from before ICs
	CourseTitle string
	Days        int // the Live Course's days, which give the PCC Level; 0 for a self-paced Course
	IssuedAt    time.Time
	Revoked     bool
	CurrentName string // the Learner's name now, which a corrected Certificate copies

	GivenByAdmin bool // the Admin gave it by hand; only the Admin Certificates list shows this
}

// certificateColumns must be scanned by queryCertificates. The subquery needs the table unaliased as certificates.
const certificateColumns = `id, year, number, learner_id, learner_name, COALESCE(learner_ic, ''), course_title,
	COALESCE(days, 0), issued_at, revoked_at IS NOT NULL,
	(SELECT name FROM learners WHERE learners.id = certificates.learner_id), given_by_admin`

// Number is the Certificate No., as PCC-2026-00509. The ID, not this, goes in URLs and the QR, so it can't be walked.
func (c certificate) Number() string {
	return fmt.Sprintf("PCC-%d-%05d", c.Year, c.Seq)
}

// MaskedIC is the Certificate's IC, formatted, with its last 4 characters hidden, as 951206-08-****.
func (c certificate) MaskedIC() string {
	ic := formatIC(c.LearnerIC)
	n := max(len(ic)-4, 0)
	return ic[:n] + strings.Repeat("*", len(ic)-n)
}

// Level is the PCC Level the Certificate was issued at, or "" for a self-paced Course.
func (c certificate) Level() string { return course{Days: c.Days}.Level() }

// Badge is the static file of the Level's badge, or "" for a self-paced Course.
func (c certificate) Badge() string { return course{Days: c.Days}.Badge() }

// IssuedIn is the issue date written in lang, as the Learner's pages, the check page, the PDF and the Admin list show it.
func (c certificate) IssuedIn(lang string) string {
	return text.Date(lang, c.IssuedAt.UTC())
}

// certificateSettings are the details the Admin sets once. LogoKey and SignatureKey are empty when unset.
type certificateSettings struct {
	OrganisationName string
	SignatoryName    string
	LogoKey          string
	SignatureKey     string
}

// newCertificateID returns 16 random bytes as an unguessable URL-safe ID.
func newCertificateID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("make certificate id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// issueCertificateIfEarned issues the Learner a Certificate for a Course once they have passed every Exam it has
// (an Exam with no published questions can't be sat, so it doesn't count) and, for a self-paced Course, completed
// every published Lesson. A Live Course needs at least one Exam passed; a self-paced one with no Exam needs only its
// Lessons. It is called when a Result is released, never at submit, so a Certificate can't give a Result away.
func (a *App) issueCertificateIfEarned(ctx context.Context, learnerID, courseID int64) error {
	var live bool
	var exams, passed int
	err := a.db.QueryRowContext(ctx, `
		SELECT c.days IS NOT NULL, count(e.id), count(e.id) FILTER (WHERE EXISTS (SELECT 1 FROM exam_attempts t
			WHERE t.exam_id = e.id AND t.learner_id = $1 AND t.passed))
		FROM courses c LEFT JOIN exams e ON e.course_id = c.id
			AND EXISTS (SELECT 1 FROM exam_questions q WHERE q.exam_id = e.id AND NOT q.is_draft)
		WHERE c.id = $2 GROUP BY c.id`, learnerID, courseID).Scan(&live, &exams, &passed)
	if errors.Is(err, sql.ErrNoRows) { // the Course is gone
		return nil
	}
	if err != nil || passed < exams || (live && exams == 0) {
		return err
	}
	if !live {
		done, total, err := a.courseProgress(ctx, learnerID, courseID)
		if err != nil || total == 0 || done < total {
			return err
		}
	}
	return a.issueCertificate(ctx, learnerID, courseID, false)
}

// issueCertificate issues the Learner a Certificate for the Course, copying their name and IC and the Course's title
// and days, unless they have no IC or already have one for it, valid or revoked: a revoked Certificate is replaced
// only by the Admin's reissue. Every Certificate is issued through here, once its rule is met, so calling it again
// changes nothing. givenByAdmin records that the Admin gave it, skipping the Exam check.
func (a *App) issueCertificate(ctx context.Context, learnerID, courseID int64, givenByAdmin bool) error {
	id, err := newCertificateID()
	if err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at, year, number, err := a.numberCertificate(ctx, tx)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO certificates (id, learner_id, course_id, learner_name, learner_ic, course_title, days, issued_at, year, number, given_by_admin)
		SELECT $1, l.id, c.id, l.name, l.ic, c.title, c.days, $4, $5, $6, $7 FROM learners l, courses c
		WHERE l.id = $2 AND c.id = $3 AND l.ic IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM certificates WHERE learner_id = $2 AND course_id = $3)
		ON CONFLICT (learner_id, course_id) WHERE revoked_at IS NULL DO NOTHING`, id, learnerID, courseID, at, year, number, givenByAdmin)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err // nothing issued: the rollback gives the number back
	}
	return tx.Commit()
}

// numberCertificate takes the next Certificate No. for the current year (UTC, as the issue date is shown) inside tx,
// with the issue time it belongs to. The counter row stays locked until tx ends, so numbers run in issue order, and a
// rolled-back issue leaves no gap.
func (a *App) numberCertificate(ctx context.Context, tx *sql.Tx) (at time.Time, year, number int, err error) {
	at = a.now().UTC()
	year = at.Year()
	err = tx.QueryRowContext(ctx, `INSERT INTO certificate_numbers (year, last) VALUES ($1, 1)
		ON CONFLICT (year) DO UPDATE SET last = certificate_numbers.last + 1 RETURNING last`, year).Scan(&number)
	return at, year, number, err
}

// issueEarnedCertificates runs issueCertificateIfEarned for every enrolment of the Learner in the Course, where 0
// means any: for one Course's roster when the Admin removes a Lesson or Exam question and so may complete the Course
// for some of them, or for one Learner's Courses when they set their IC.
// ponytail: a few queries per enrolment (N+1); fine for one Course's roster, batch it if Courses grow past a few hundred.
func (a *App) issueEarnedCertificates(ctx context.Context, learnerID, courseID int64) error {
	rows, err := a.db.QueryContext(ctx, `SELECT learner_id, course_id FROM enrollments
		WHERE ($1::bigint = 0 OR learner_id = $1) AND ($2::bigint = 0 OR course_id = $2)`, learnerID, courseID)
	if err != nil {
		return err
	}
	var pairs [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, p)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range pairs {
		if err := a.issueCertificateIfEarned(ctx, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

// queryCertificates runs a query that selects certificateColumns and returns the Certificates it finds.
func (a *App) queryCertificates(ctx context.Context, query string, args ...any) ([]certificate, error) {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []certificate
	for rows.Next() {
		var c certificate
		if err := rows.Scan(&c.ID, &c.Year, &c.Seq, &c.LearnerID, &c.LearnerName, &c.LearnerIC, &c.CourseTitle, &c.Days, &c.IssuedAt, &c.Revoked, &c.CurrentName, &c.GivenByAdmin); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// certificateStatus is the Certificate state the Admin sees, in lang, for a Learner on a Course: "Issued" with the
// date of the valid Certificate, "Revoked" when only revoked ones exist, or "None".
func (a *App) certificateStatus(ctx context.Context, lang string, learnerID, courseID int64) (string, error) {
	var issued time.Time
	var revoked sql.NullTime
	err := a.db.QueryRowContext(ctx, `SELECT issued_at, revoked_at FROM certificates
		WHERE learner_id = $1 AND course_id = $2 ORDER BY revoked_at IS NULL DESC, issued_at DESC LIMIT 1`,
		learnerID, courseID).Scan(&issued, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return text.T(lang, "adm.course.cert-none"), nil
	}
	if err != nil {
		return "", err
	}
	if revoked.Valid {
		return text.T(lang, "adm.course.cert-revoked"), nil
	}
	return text.T(lang, "adm.course.cert-issued", text.Date(lang, issued.UTC())), nil
}

// myCertificates lists the logged-in Learner's Certificates, revoked ones included, each with a download link, and
// tells a Learner with no IC that none can be issued until they set it.
func (a *App) myCertificates(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates WHERE learner_id = $1 ORDER BY issued_at, year, number`, learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ic, err := a.learnerIC(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_certificates", page{Learner: learner, Certificates: list, NeedsIC: ic == ""})
}

// downloadCertificate serves a Certificate PDF to its Learner and to the Admin. Anyone else gets Not found.
// A revoked Certificate still downloads, marked as revoked.
func (a *App) downloadCertificate(w http.ResponseWriter, r *http.Request) {
	viewer, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if viewer == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(list) == 0 || (list[0].LearnerID != viewer.ID && !viewer.IsAdmin) {
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "certificate.not-found.title"), Message: text.T(lang, "certificate.not-found-here")})
		return
	}
	pdf, err := a.certificatePDF(r.Context(), lang, list[0])
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="certificate-%s.pdf"`, list[0].ID))
	_, _ = w.Write(pdf)
}

// certificatePDF lays out the Certificate on one landscape A4 page, its fixed wording and date in lang.
func (a *App) certificatePDF(ctx context.Context, lang string, c certificate) ([]byte, error) {
	s, err := a.certificateSettingsFor(ctx)
	if err != nil {
		return nil, err
	}
	pdf := fpdf.New("L", "mm", "A4", "")
	// Uncompressed, so the text stays greppable in tests. The page is small text, so the cost is negligible.
	pdf.SetCompression(false)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.SetLineWidth(1.2)
	pdf.Rect(10, 10, 277, 190, "D")
	line := func(y float64, style string, size float64, text string) {
		pdf.SetFont("Helvetica", style, size)
		pdf.SetXY(0, y)
		pdf.CellFormat(297, size/2, tr(text), "", 0, "C", false, 0, "")
	}
	if s.LogoKey != "" {
		if err := a.placeImage(pdf, s.LogoKey, 20, 16, 0, 26); err != nil {
			return nil, err
		}
	}
	if s.OrganisationName != "" {
		line(30, "B", 18, s.OrganisationName)
	}
	if c.Revoked {
		pdf.SetTextColor(200, 0, 0)
		line(14, "B", 28, text.T(lang, "certificate.pdf.revoked"))
		line(190, "B", 12, text.T(lang, "certificate.pdf.revoked-note"))
		pdf.SetTextColor(0, 0, 0)
	}
	line(48, "B", 32, text.T(lang, "certificate.pdf.title"))
	line(72, "", 14, text.T(lang, "certificate.certifies"))
	line(84, "B", 28, c.LearnerName)
	if c.LearnerIC != "" {
		line(99, "", 12, formatIC(c.LearnerIC))
	}
	line(106, "", 14, text.T(lang, "certificate.completed"))
	line(118, "B", 22, c.CourseTitle)
	line(138, "", 12, text.T(lang, "certificate.issued", c.IssuedIn(lang)))
	line(146, "", 11, text.T(lang, "certificate.pdf.id", c.ID))
	line(154, "", 11, text.T(lang, "certificate.pdf.verify-at", a.publicBaseURL+"/certificates/"+c.ID))
	if s.SignatureKey != "" {
		if err := a.placeImage(pdf, s.SignatureKey, 40, 162, 0, 20); err != nil {
			return nil, err
		}
	}
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetXY(25, 184)
	pdf.CellFormat(90, 6, tr(s.SignatoryName), "T", 0, "C", false, 0, "")
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// placeImage draws a stored JPEG or PNG. Its type is sniffed again, since the file is only trusted as far as the upload check.
func (a *App) placeImage(pdf *fpdf.Fpdf, key string, x, y, w, h float64) error {
	f, err := a.store.Open(key)
	if err != nil {
		return fmt.Errorf("open certificate image: %w", err)
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	tp := "jpg"
	if http.DetectContentType(head[:n]) == "image/png" {
		tp = "png"
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	opt := fpdf.ImageOptions{ImageType: tp}
	pdf.RegisterImageOptionsReader(key, opt, f)
	pdf.ImageOptions(key, x, y, w, h, false, opt, 0, "")
	return pdf.Error()
}

func (a *App) certificateSettingsFor(ctx context.Context) (certificateSettings, error) {
	var s certificateSettings
	err := a.db.QueryRowContext(ctx, `SELECT organisation_name, signatory_name, COALESCE(logo_key, ''), COALESCE(signature_key, '')
		FROM certificate_settings WHERE id = 1`).
		Scan(&s.OrganisationName, &s.SignatoryName, &s.LogoKey, &s.SignatureKey)
	if err != nil {
		return s, fmt.Errorf("load certificate settings: %w", err)
	}
	return s, nil
}

// certificateSettingsForm is the Admin page for the Certificate details.
func (a *App) certificateSettingsForm(w http.ResponseWriter, r *http.Request) {
	s, err := a.certificateSettingsFor(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_certificate_settings", page{Settings: &s})
}

// saveCertificateSettings stores the text fields and any new logo or signature. Nothing is kept unless the whole form is valid.
func (a *App) saveCertificateSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := a.certificateSettingsFor(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCertificateImage)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.render(w, r, http.StatusRequestEntityTooLarge, "admin_certificate_settings", page{Settings: &cur,
				Error: text.T(lang, "adm.certs.err.too-large")})
			return
		}
		a.render(w, r, http.StatusBadRequest, "admin_certificate_settings", page{Settings: &cur,
			Error: text.T(lang, "adm.certs.err.unread")})
		return
	}
	next := certificateSettings{
		OrganisationName: strings.TrimSpace(r.PostFormValue("organisation_name")),
		SignatoryName:    strings.TrimSpace(r.PostFormValue("signatory_name")),
	}
	var saved []string // keys stored by this request, removed again unless the save commits
	committed := false
	defer func() {
		if !committed {
			for _, key := range saved {
				_ = a.store.Delete(r.Context(), key)
			}
		}
	}()
	fail := func(msg string) {
		a.render(w, r, http.StatusUnprocessableEntity, "admin_certificate_settings", page{Settings: &cur, Error: msg})
	}
	logo, msg, err := a.saveCertificateImage(r, lang, "logo")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if logo != "" {
		saved = append(saved, logo)
	}
	if msg != "" {
		fail(msg)
		return
	}
	signature, msg, err := a.saveCertificateImage(r, lang, "signature")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if signature != "" {
		saved = append(saved, signature)
	}
	if msg != "" {
		fail(msg)
		return
	}
	if next.OrganisationName == "" || next.SignatoryName == "" {
		fail(text.T(lang, "adm.certs.err.names"))
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE certificate_settings SET organisation_name = $1, signatory_name = $2,
		logo_key = COALESCE(NULLIF($3, ''), logo_key), signature_key = COALESCE(NULLIF($4, ''), signature_key) WHERE id = 1`,
		next.OrganisationName, next.SignatoryName, logo, signature)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	committed = true
	for _, pair := range [][2]string{{logo, cur.LogoKey}, {signature, cur.SignatureKey}} {
		if pair[0] != "" && pair[1] != "" {
			_ = a.store.Delete(r.Context(), pair[1]) // the image it replaces
		}
	}
	http.Redirect(w, r, "/admin/certificate-settings", http.StatusSeeOther)
}

// saveCertificateImage stores the uploaded image in field ("logo" or "signature"), if any. msg is set, in lang,
// when the file is not a JPEG or PNG.
func (a *App) saveCertificateImage(r *http.Request, lang, field string) (key, msg string, err error) {
	f, _, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	if ct := http.DetectContentType(head[:n]); ct != "image/png" && ct != "image/jpeg" {
		return "", text.T(lang, "adm.certs.err.image-"+field), nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	key, _, err = a.store.Save(f)
	return key, "", err
}
