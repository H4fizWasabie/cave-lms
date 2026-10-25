package app

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/jackc/pgx/v5/pgconn"
)

// verifyCertificateForm is the public check page. An ID typed into it goes to that Certificate's verification page.
func (a *App) verifyCertificateForm(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		http.Redirect(w, r, "/certificates/"+url.PathEscape(id), http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "certificate_verify", page{})
}

// searchCertificates is the check page's search, by a full IC or passport number, matched against the Certificate's
// copy, or a Certificate No. It is posted, so the IC is in no URL, and the results render from the post.
func (a *App) searchCertificates(w http.ResponseWriter, r *http.Request) {
	q := normaliseIC(r.PostFormValue("q"))
	if q == "" {
		a.render(w, r, http.StatusOK, "certificate_verify", page{})
		return
	}
	if !a.limiter.allow("certificate search "+clientAddress(r), certificateSearches, rateLimitWindow, a.now()) {
		a.tooManyAttempts(w, r)
		return
	}
	var year, number int
	if m := certificateNumber.FindStringSubmatch(q); m != nil {
		year, _ = strconv.Atoi(m[1])
		number, _ = strconv.Atoi(m[2])
	}
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates
		WHERE learner_ic = $1 OR (year = $2 AND number = $3) ORDER BY issued_at DESC, year DESC, number DESC`, q, year, number)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "certificate_verify", page{Certificates: list, Query: q})
}

// certificateNumber is a Certificate No. after normaliseIC: PCC-2026-00509 becomes PCC202600509.
var certificateNumber = regexp.MustCompile(`^PCC(\d{4})(\d{5,9})$`)

// verifyCertificate is public. It shows the name, Course, PCC Level, issue date and status of a Certificate, and
// nothing else.
func (a *App) verifyCertificate(w http.ResponseWriter, r *http.Request) {
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(list) == 0 {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "certificate.not-found.title"), Message: text.T(lang, "certificate.no-such-id")})
		return
	}
	a.render(w, r, http.StatusOK, "certificate_verify", page{Certificates: list[:1]})
}

// adminCertificates lists every Certificate, newest first. ?q= matches the Learner's name or the exact ID.
func (a *App) adminCertificates(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates
		WHERE $1 = '' OR id = $1 OR strpos(lower(learner_name), lower($1)) > 0
		ORDER BY issued_at DESC, year DESC, number DESC`, q)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := page{Certificates: list, Query: q}
	if r.URL.Query().Has("already-revoked") {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Message = text.T(lang, "adm.certs.already-revoked")
	}
	a.render(w, r, http.StatusOK, "admin_certificates", p)
}

// confirmRevokeCertificate asks the Admin to confirm revoking a Certificate.
func (a *App) confirmRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(list) == 0 {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "adm.certs.not-found.title"), Message: text.T(lang, "adm.certs.not-found")})
		return
	}
	if list[0].Revoked {
		http.Redirect(w, r, "/admin/certificates?already-revoked", http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "revoke_certificate", page{Certificate: &list[0]})
}

// revokeCertificate stops a Certificate counting as valid, once its confirmation page is posted; a post without it goes
// to that page instead. From then on its check page, the Learner's list and its PDF show it as revoked.
func (a *App) revokeCertificate(w http.ResponseWriter, r *http.Request) {
	if !confirmed(r) {
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE certificates SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, r.PathValue("id"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/certificates", http.StatusSeeOther)
}

// reissueCertificate issues a corrected Certificate in place of a revoked one. The name the Admin gives becomes the
// Learner's name, and the new Certificate, with a new ID, copies it, the Learner's current IC and the Course's current
// title, days and Organisation's logo, and is emailed to them. A Learner with no IC gets none. The partial unique index refuses it while a
// valid one exists, and then the name is not changed either.
func (a *App) reissueCertificate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	old, err := a.queryCertificates(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(old) == 0 {
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "adm.certs.not-found.title"), Message: text.T(lang, "adm.certs.not-found")})
		return
	}
	notReissued := func(status int, key string) {
		a.render(w, r, status, "message", page{Title: text.T(lang, "adm.certs.not-reissued"), Message: text.T(lang, key)})
	}
	if !old[0].Revoked {
		notReissued(http.StatusUnprocessableEntity, "adm.certs.err.revoke-first")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("learner_name"))
	if name == "" || len(name) > 200 {
		notReissued(http.StatusUnprocessableEntity, "adm.certs.err.name")
		return
	}
	if ic, err := a.learnerIC(ctx, old[0].LearnerID); err != nil {
		a.serverError(w, r, err)
		return
	} else if ic == "" {
		a.render(w, r, http.StatusUnprocessableEntity, "message", page{Title: text.T(lang, "adm.certs.not-reissued"), Message: text.T(lang, "adm.certs.err.no-ic", name)})
		return
	}
	id, err := newCertificateID()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE learners SET name = $2 WHERE id = $1`, old[0].LearnerID, name)
	if err == nil {
		var at time.Time
		var year, number int
		if at, year, number, err = a.numberCertificate(ctx, tx); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO certificates (id, learner_id, course_id, learner_name, learner_ic, course_title, days, issued_at, year, number, logo_key)
				SELECT $1, l.id, c.course_id, l.name, l.ic, co.title, co.days, $3, $4, $5, o.logo_key FROM certificates c
				JOIN learners l ON l.id = c.learner_id JOIN courses co ON co.id = c.course_id
				LEFT JOIN organisations o ON o.id = co.organisation_id
				WHERE c.id = $2`, id, old[0].ID, at, year, number)
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if isUniqueViolation(err) {
		notReissued(http.StatusConflict, "adm.certs.err.has-valid")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.emailCertificate(ctx, id)
	http.Redirect(w, r, "/admin/certificates?q="+url.QueryEscape(id), http.StatusSeeOther)
}

// isUniqueViolation reports whether err is a violated unique constraint (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
