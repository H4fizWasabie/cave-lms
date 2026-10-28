package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/go-pdf/fpdf"
)

// maxCertificateImage caps the form that carries the background image, which is a full-page print-quality artwork.
const maxCertificateImage = 20 << 20

// certificate is one issued Certificate. LearnerName, LearnerIC, CourseTitle, Days and LogoKey are copies taken at issue time.
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

	GivenByAdmin bool   // the Admin gave it by hand; only the Admin Certificates list shows this
	LogoKey      string // the Live Course's Organisation's logo, "" for none
}

// certificateColumns must be scanned by queryCertificates. The subquery needs the table unaliased as certificates.
const certificateColumns = `id, year, number, learner_id, learner_name, COALESCE(learner_ic, ''), course_title,
	COALESCE(days, 0), issued_at, revoked_at IS NOT NULL,
	(SELECT name FROM learners WHERE learners.id = certificates.learner_id), given_by_admin, COALESCE(logo_key, '')`

// Number is the Certificate No., as PCC-LMS-260001: the year's last two digits and its place in the year, four digits
// or more. The ID, not this, goes in URLs and the QR, so it can't be walked.
func (c certificate) Number() string {
	return fmt.Sprintf("PCC-LMS-%02d%04d", c.Year%100, c.Seq)
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
	return text.Date(lang, c.IssuedAt.In(malaysia))
}

// certificateSettings are what the Admin sets once: the storage key of the background artwork, empty when unset.
type certificateSettings struct {
	BackgroundKey string
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
// (an Exam with no published questions can't be sat, so it doesn't count), Lessons or not, self-paced or Live. A
// self-paced Course with no Exam needs every published Lesson completed instead; a Live one with no Exam issues none. It is called when a Result is released, never at submit, so a Certificate can't give a Result away.
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
	if err != nil || passed < exams {
		return err
	}
	if exams == 0 {
		if live {
			return nil
		}
		done, total, err := a.courseProgress(ctx, learnerID, courseID)
		if err != nil || total == 0 || done < total {
			return err
		}
	}
	return a.issueCertificate(ctx, learnerID, courseID, false)
}

// issueCertificate issues the Learner a Certificate for the Course, copying their name and IC and the Course's title,
// days and Organisation's logo, unless they have no IC or already have one for it, valid or revoked: a revoked Certificate is replaced
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
		INSERT INTO certificates (id, learner_id, course_id, learner_name, learner_ic, course_title, days, issued_at, year, number, given_by_admin, logo_key)
		SELECT $1, l.id, c.id, l.name, l.ic, c.title, c.days, $4, $5, $6, $7, o.logo_key FROM learners l, courses c
		LEFT JOIN organisations o ON o.id = c.organisation_id
		WHERE l.id = $2 AND c.id = $3 AND l.ic IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM certificates WHERE learner_id = $2 AND course_id = $3)
		ON CONFLICT (learner_id, course_id) WHERE revoked_at IS NULL DO NOTHING`, id, learnerID, courseID, at, year, number, givenByAdmin)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err // nothing issued: the rollback gives the number back
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.emailCertificate(ctx, id)
	return nil
}

// emailCertificate emails a just-issued Certificate to its Learner in their saved language, with its PDF and a link
// to its check page. A failed email is only logged: the Certificate is on the site.
func (a *App) emailCertificate(ctx context.Context, id string) {
	var email, lang string
	list, err := a.queryCertificates(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, id)
	if err == nil && len(list) == 1 {
		err = a.db.QueryRowContext(ctx, `SELECT email, COALESCE(language, '') FROM learners WHERE id = $1`, list[0].LearnerID).Scan(&email, &lang)
	}
	if err == nil && len(list) != 1 {
		err = errors.New("not found")
	}
	if err != nil {
		a.logError(nil, fmt.Errorf("send Certificate email for %s: %w", id, err))
		return
	}
	c := list[0]
	if !text.Supported(lang) {
		lang = text.Languages[0]
	}
	msg := mailer.Message{
		To:      email,
		Subject: text.T(lang, "email.certificate.subject", c.CourseTitle),
		Body:    text.T(lang, "email.certificate.body", c.LearnerName, c.CourseTitle, c.Number(), a.publicBaseURL+"/certificates/"+c.ID),
	}
	if pdf, err := a.certificatePDF(ctx, c); err != nil {
		a.logError(nil, fmt.Errorf("make Certificate PDF for %s: %w", c.ID, err))
	} else {
		msg.Body += "\n" + text.T(lang, "email.certificate.attached") + "\n"
		msg.Attachments = []mailer.Attachment{{Filename: "certificate-" + c.Number() + ".pdf", Content: pdf}}
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		a.logError(nil, fmt.Errorf("send Certificate email for %s: %w", c.ID, err))
	}
}

// numberCertificate takes the next Certificate No. for the current year (Malaysia time, as the issue date is shown) inside tx,
// with the issue time it belongs to. The counter row stays locked until tx ends, so numbers run in issue order, and a
// rolled-back issue leaves no gap.
func (a *App) numberCertificate(ctx context.Context, tx *sql.Tx) (at time.Time, year, number int, err error) {
	at = a.now()
	year = at.In(malaysia).Year()
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
		if err := rows.Scan(&c.ID, &c.Year, &c.Seq, &c.LearnerID, &c.LearnerName, &c.LearnerIC, &c.CourseTitle, &c.Days, &c.IssuedAt, &c.Revoked, &c.CurrentName, &c.GivenByAdmin, &c.LogoKey); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// certificateStatus is the Certificate state the Admin sees, in lang, for a Learner on a Course: "Issued" with the
// date of the valid Certificate, "Revoked" when only revoked ones exist, or "None". It also returns that Certificate's
// ID, "" for "None".
func (a *App) certificateStatus(ctx context.Context, lang string, learnerID, courseID int64) (status, id string, err error) {
	var issued time.Time
	var revoked sql.NullTime
	err = a.db.QueryRowContext(ctx, `SELECT id, issued_at, revoked_at FROM certificates
		WHERE learner_id = $1 AND course_id = $2 ORDER BY revoked_at IS NULL DESC, issued_at DESC LIMIT 1`,
		learnerID, courseID).Scan(&id, &issued, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return text.T(lang, "adm.course.cert-none"), "", nil
	}
	if err != nil {
		return "", "", err
	}
	if revoked.Valid {
		return text.T(lang, "adm.course.cert-revoked"), id, nil
	}
	return text.T(lang, "adm.course.cert-issued", text.Date(lang, issued.In(malaysia))), id, nil
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
	pdf, err := a.certificatePDF(r.Context(), list[0])
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="certificate-%s.pdf"`, list[0].ID))
	_, _ = w.Write(pdf)
}

// The Certificate's layout on A4 portrait, in mm, after the client's artwork for sample PCC-2026-00509, as its
// uploaded background lays it out. Baselines are where the artwork, built-in or uploaded, leaves room for the details.
const (
	certNumberX, certNumberY   = 183.0, 19.9 // centre and baseline of the Certificate No., under its label
	certLabelY, certLabelPt    = 15.0, 7.5   // the label's baseline and font size
	certQRX, certQRY, certQR   = 173.2, 21.1, 19.3
	certNameY, certICY         = 96.0, 101.0  // centred on the page, over the rule at 104.7
	certNameW                  = 160.0        // a longer name shrinks to this width, inside the 168 mm rule
	certTitleX, certTitleW     = 30.0, 150.0  // the title box, under "has successfully completed..."
	certTitleY, certTitleH     = 115.0, 44.0  // above the ornament; the title is centred in it, shrunk until it fits
	certBadgeX, certBadgeY     = 102.0, 209.6 // the badge image's top left; its shield spans 108-133 mm, as on the sample
	certBadgeW, certBadgeH     = 36.7, 52.1   // the badge files are 904 by 1280
	certIssuedX, certIssuedY   = 18.2, 281.7  // "Issued on" bottom left
	certNumberPt, certIssuedPt = 8.0, 6.5     // the Certificate No.'s and the issue date's font sizes
	// The Organisation's logo is scaled to fit this box and centred in it: left of the CAVE logo, where the client's
	// artwork leaves room for a partner's logo, 7 mm clear of it.
	certLogoX, certLogoY = 45.0, 10.0
	certLogoW, certLogoH = 55.0, 30.0
)

// certificatePDF prints the Certificate's details on one portrait A4 page, in English like the artwork, over the
// Admin's uploaded background or, without one, over the built-in artwork.
func (a *App) certificatePDF(ctx context.Context, c certificate) ([]byte, error) {
	s, err := a.certificateSettingsFor(ctx)
	if err != nil {
		return nil, err
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	// Uncompressed, so the text stays greppable in tests. The images are compressed on their own anyway.
	pdf.SetCompression(false)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()
	pageW, pageH := pdf.GetPageSize()
	if s.BackgroundKey != "" {
		err = a.placeImage(pdf, s.BackgroundKey, 0, 0, pageW, pageH)
	} else {
		err = drawArtwork(pdf)
	}
	if err != nil {
		return nil, err
	}
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	// centred writes s with its baseline at y, centred on x.
	centred := func(x, y float64, family, style string, size float64, s string) {
		pdf.SetFont(family, style, size)
		s = tr(s)
		pdf.Text(x-pdf.GetStringWidth(s)/2, y, s)
	}

	pdf.SetTextColor(artInk[0], artInk[1], artInk[2])
	centred(certNumberX, certLabelY, "Helvetica", "B", certLabelPt, certNumberLabel)
	pdf.SetTextColor(artNumber[0], artNumber[1], artNumber[2])
	centred(certNumberX, certNumberY, "Helvetica", "", certNumberPt, c.Number())
	pdf.SetTextColor(artInk[0], artInk[1], artInk[2])
	checkURL := a.publicBaseURL + "/certificates/" + c.ID
	if err := placeQR(pdf, checkURL, certQRX, certQRY, certQR); err != nil {
		return nil, err
	}
	pdf.LinkString(certQRX, certQRY, certQR, certQR, checkURL)

	name := strings.ToUpper(c.LearnerName) // in capitals, as the sample prints it; the stored name is as typed
	centred(pageW/2, certNameY, "Times", "B", certNameSize(pdf, name), name)
	if c.LearnerIC != "" {
		centred(pageW/2, certICY, "Times", "", 10, formatIC(c.LearnerIC))
	}
	size, lines := certTitleLines(pdf, c.CourseTitle)
	for _, l := range lines {
		centred(certTitleX+certTitleW/2, l.Y, "Times", "B", size, l.Text)
	}

	if c.LogoKey != "" {
		info, err := a.registerImage(pdf, c.LogoKey)
		if err != nil {
			return nil, err
		}
		iw, ih := info.Extent()
		scale := min(certLogoW/iw, certLogoH/ih)
		w, h := iw*scale, ih*scale
		pdf.ImageOptions(c.LogoKey, certLogoX+(certLogoW-w)/2, certLogoY+(certLogoH-h)/2, w, h, false, fpdf.ImageOptions{}, 0, "")
	}
	if badge := c.Badge(); badge != "" {
		cut, err := badgeCutouts()
		if err != nil {
			return nil, err
		}
		opt := fpdf.ImageOptions{ImageType: "png"}
		pdf.RegisterImageOptionsReader(badge, opt, bytes.NewReader(cut[badge]))
		pdf.ImageOptions(badge, certBadgeX, certBadgeY, certBadgeW, certBadgeH, false, opt, 0, "")
	}
	pdf.SetFont("Helvetica", "", certIssuedPt)
	pdf.Text(certIssuedX, certIssuedY, certIssuedOn+c.IssuedIn("en"))

	if c.Revoked {
		pdf.SetFillColor(255, 255, 255)
		pdf.Rect(20, 52, pageW-40, 27, "F") // over the heading, so the mark reads clearly on any artwork
		pdf.SetTextColor(200, 0, 0)
		centred(pageW/2, 66, "Helvetica", "B", 40, certRevoked)
		centred(pageW/2, 74, "Helvetica", "B", 12, certRevokedNote)
	}
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// placeQR draws a QR code of content as a size mm square at x, y.
func placeQR(pdf *fpdf.Fpdf, content string, x, y, size float64) error {
	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return err
	}
	n := code.Bounds().Dx()
	code, err = barcode.Scale(code, n*8, n*8) // whole pixels per module, so the PDF viewer's smoothing keeps edges sharp
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, code); err != nil {
		return err
	}
	opt := fpdf.ImageOptions{ImageType: "png"}
	pdf.RegisterImageOptionsReader("qr", opt, &buf)
	pdf.ImageOptions("qr", x, y, size, size, false, opt, 0, "")
	return pdf.Error()
}

// placeImage draws a stored JPEG or PNG at x, y, w by h.
func (a *App) placeImage(pdf *fpdf.Fpdf, key string, x, y, w, h float64) error {
	if _, err := a.registerImage(pdf, key); err != nil {
		return err
	}
	pdf.ImageOptions(key, x, y, w, h, false, fpdf.ImageOptions{}, 0, "")
	return pdf.Error()
}

// registerImage loads a stored JPEG or PNG into pdf under its key. Its type is sniffed again, since the file is only
// trusted as far as the upload check.
func (a *App) registerImage(pdf *fpdf.Fpdf, key string) (*fpdf.ImageInfoType, error) {
	f, err := a.store.Open(key)
	if err != nil {
		return nil, fmt.Errorf("open stored image: %w", err)
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	tp := "jpg"
	if http.DetectContentType(head[:n]) == "image/png" {
		tp = "png"
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	opt := fpdf.ImageOptions{ImageType: tp}
	info := pdf.RegisterImageOptionsReader(key, opt, f)
	return info, pdf.Error()
}

// certBackgroundURL is the public address of the uploaded background with storage key key, "" for none.
func certBackgroundURL(key string) string {
	if key == "" {
		return ""
	}
	return "/certificate-background/" + key
}

func (a *App) certificateSettingsFor(ctx context.Context) (certificateSettings, error) {
	var s certificateSettings
	err := a.db.QueryRowContext(ctx, `SELECT COALESCE(background_key, '') FROM certificate_settings WHERE id = 1`).Scan(&s.BackgroundKey)
	if err != nil {
		return s, fmt.Errorf("load certificate settings: %w", err)
	}
	return s, nil
}

// certificateSettingsForm is the Admin page for the Certificate background.
func (a *App) certificateSettingsForm(w http.ResponseWriter, r *http.Request) {
	s, err := a.certificateSettingsFor(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_certificate_settings", page{Settings: &s})
}

// certificateBackground serves the background image: to the Admin at /admin/certificate-settings/background, so the
// settings page can show it, and to everyone at certBackgroundURL, which the Certificate preview draws. The image is
// the artwork every Certificate shares, with nothing about any Learner. Under /certificate-background/{key} only the
// current key is found, so a replaced background's address stops working and browsers may keep the image.
func (a *App) certificateBackground(w http.ResponseWriter, r *http.Request) {
	s, err := a.certificateSettingsFor(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if key := r.PathValue("key"); s.BackgroundKey == "" || (key != "" && key != s.BackgroundKey) {
		a.notFound(w, r)
		return
	}
	a.serveImage(w, r, s.BackgroundKey)
}

// serveImage serves a stored image as the type its content is sniffed as. Only JPEG and PNG pass the upload checks.
func (a *App) serveImage(w http.ResponseWriter, r *http.Request, key string) {
	f, err := a.store.Open(key)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	f.Close()
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		a.serverError(w, r, err)
		return
	}
	a.serveStored(w, r, key, http.DetectContentType(head[:n]))
}

// saveCertificateSettings stores a new background image, if one was sent, and removes the one it replaces.
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
	background, msg, err := a.saveCertificateImage(r, lang, "background")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "admin_certificate_settings", page{Settings: &cur, Error: msg})
		return
	}
	if background == "" {
		http.Redirect(w, r, "/admin/certificate-settings", http.StatusSeeOther)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE certificate_settings SET background_key = $1 WHERE id = 1`, background); err != nil {
		_ = a.store.Delete(r.Context(), background)
		a.serverError(w, r, err)
		return
	}
	if cur.BackgroundKey != "" {
		_ = a.store.Delete(r.Context(), cur.BackgroundKey) // the image it replaces
	}
	http.Redirect(w, r, "/admin/certificate-settings", http.StatusSeeOther)
}

// saveCertificateImage stores the uploaded image in field, if any. msg is set, in lang, when the file is not a JPEG or PNG.
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
