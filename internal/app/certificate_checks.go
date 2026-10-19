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
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
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
	list, err := a.queryCertificates(r.Context(), `SELECT `+certificateColumns+` FROM certificates
		WHERE $1 = '' OR id = $1 OR strpos(lower(learner_name), lower($1)) > 0
		ORDER BY issued_at DESC, id`, q)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "admin_certificates", page{Certificates: list, Query: q})
}

// revokeCertificate stops a Certificate counting as valid. From then on its check page, the Learner's list and its PDF
// show it as revoked.
func (a *App) revokeCertificate(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.ExecContext(r.Context(), `UPDATE certificates SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, r.PathValue("id"))
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/certificates", http.StatusSeeOther)
}

// reissueCertificate issues a corrected Certificate in place of a revoked one. The name the Admin gives becomes the
// Learner's name, and the new Certificate, with a new ID, copies it and the Course's current title. The partial
// unique index refuses it while a valid one exists, and then the name is not changed either.
func (a *App) reissueCertificate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	old, err := a.queryCertificates(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, r.PathValue("id"))
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
	if name == "" || len(name) > 200 {
		a.render(w, http.StatusUnprocessableEntity, "message", page{Title: "Certificate not reissued", Message: "Enter a name for the Certificate (up to 200 characters)."})
		return
	}
	id, err := newCertificateID()
	if err != nil {
		serverError(w, err)
		return
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE learners SET name = $2 WHERE id = $1`, old[0].LearnerID, name)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO certificates (id, learner_id, course_id, learner_name, course_title)
			SELECT $1, l.id, c.course_id, l.name, co.title FROM certificates c
			JOIN learners l ON l.id = c.learner_id JOIN courses co ON co.id = c.course_id
			WHERE c.id = $2`, id, old[0].ID)
	}
	if err == nil {
		err = tx.Commit()
	}
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
