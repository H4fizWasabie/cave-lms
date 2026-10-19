package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// verifyCertificateForm is the public ID box. An ID typed into it goes to that Certificate's verification page.
func (a *App) verifyCertificateForm(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		http.Redirect(w, r, "/certificates/"+url.PathEscape(id), http.StatusSeeOther)
		return
	}
	a.render(w, http.StatusOK, "certificate_verify", page{})
}

// verifyCertificate is public. It shows the name, Course, issue date and status of a Certificate, and nothing else.
func (a *App) verifyCertificate(w http.ResponseWriter, r *http.Request) {
	list, err := a.queryCertificates(r.Context(), `SELECT id, learner_id, learner_name, course_title, issued_at, revoked_at IS NOT NULL
		FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		serverError(w, err)
		return
	}
	if len(list) == 0 {
		a.render(w, http.StatusNotFound, "message", page{Title: "Certificate not found", Message: "No Certificate has this ID."})
		return
	}
	a.render(w, http.StatusOK, "certificate_verify", page{Certificate: &list[0]})
}

// adminCertificates lists every Certificate, newest first. ?q= matches the Learner's name or the exact ID.
func (a *App) adminCertificates(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list, err := a.queryCertificates(r.Context(), `SELECT id, learner_id, learner_name, course_title, issued_at, revoked_at IS NOT NULL
		FROM certificates WHERE $1 = '' OR id = $1 OR strpos(lower(learner_name), lower($1)) > 0
		ORDER BY issued_at DESC, id`, q)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "admin_certificates", page{Certificates: list, Query: q})
}

// revokeCertificate stops a Certificate counting as valid. It leaves the Learner's list and downloads at once.
func (a *App) revokeCertificate(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.ExecContext(r.Context(), `UPDATE certificates SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, r.PathValue("id"))
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/certificates", http.StatusSeeOther)
}

// reissueCertificate issues a corrected Certificate in place of a revoked one. The new one has a new ID, the name
// the Admin gives and the Course's current title. The partial unique index refuses it while a valid one exists.
func (a *App) reissueCertificate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	old, err := a.queryCertificates(ctx, `SELECT id, learner_id, learner_name, course_title, issued_at, revoked_at IS NOT NULL
		FROM certificates WHERE id = $1`, r.PathValue("id"))
	if err != nil {
		serverError(w, err)
		return
	}
	if len(old) == 0 {
		a.render(w, http.StatusNotFound, "message", page{Title: "Certificate not found", Message: "No Certificate has this ID."})
		return
	}
	if !old[0].Revoked {
		a.render(w, http.StatusUnprocessableEntity, "message", page{Title: "Certificate not reissued", Message: "Revoke the Certificate before issuing a corrected one."})
		return
	}
	name := strings.TrimSpace(r.PostFormValue("learner_name"))
	if name == "" {
		a.render(w, http.StatusUnprocessableEntity, "message", page{Title: "Certificate not reissued", Message: "Enter a name for the Certificate."})
		return
	}
	id, err := newCertificateID()
	if err != nil {
		serverError(w, err)
		return
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO certificates (id, learner_id, course_id, learner_name, course_title)
		SELECT $1, c.learner_id, c.course_id, $2, co.title FROM certificates c JOIN courses co ON co.id = c.course_id
		WHERE c.id = $3`, id, name, old[0].ID)
	if isUniqueViolation(err) {
		a.render(w, http.StatusConflict, "message", page{Title: "Certificate not reissued", Message: "This Learner already has a valid Certificate for this Course."})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/certificates?q="+url.QueryEscape(id), http.StatusSeeOther)
}

// isUniqueViolation reports whether err is a violated unique constraint (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
