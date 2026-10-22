package app

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// programme is a Programme: an upcoming programme the Admin announces. PictureKey is empty when it has no picture.
// Its picture sits in the same storage as Gallery files but is not a Gallery item.
type programme struct {
	ID          int64
	Title       string
	StartsOn    time.Time
	Blurb       string
	Link        string
	PictureKey  string
	pictureType string
}

// Starts is the start date in lang.
func (p programme) Starts(lang string) string { return text.Date(lang, p.StartsOn) }

// StartsValue is the start date as a date input's value.
func (p programme) StartsValue() string {
	if p.StartsOn.IsZero() {
		return ""
	}
	return p.StartsOn.Format(time.DateOnly)
}

type programmesPage struct {
	pageBase
	Programmes []programme
}

type programmeFormPage struct {
	pageBase
	Programme programme
}

const programmeColumns = `id, title, starts_on, blurb, link, picture_key, picture_type`

func (a *App) listProgrammes(r *http.Request, query string) ([]programme, error) {
	rows, err := a.db.QueryContext(r.Context(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []programme
	for rows.Next() {
		var p programme
		if err := rows.Scan(&p.ID, &p.Title, &p.StartsOn, &p.Blurb, &p.Link, &p.PictureKey, &p.pictureType); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// programmes is the public Program page: Programmes starting today or later in Malaysia, soonest first.
func (a *App) programmes(w http.ResponseWriter, r *http.Request) {
	list, err := a.listProgrammes(r, `SELECT `+programmeColumns+` FROM programmes
		WHERE starts_on >= (now() AT TIME ZONE 'Asia/Kuala_Lumpur')::date ORDER BY starts_on, id`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "programmes", &programmesPage{Programmes: list})
}

// programmePicture serves a Programme's picture by its storage key; Gallery files are not served here.
func (a *App) programmePicture(w http.ResponseWriter, r *http.Request) {
	var contentType string
	err := a.db.QueryRowContext(r.Context(), `SELECT picture_type FROM programmes WHERE picture_key = $1 AND picture_key <> ''`, r.PathValue("key")).Scan(&contentType)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.serveStored(w, r, r.PathValue("key"), contentType)
}

// adminProgrammes lists every Programme, past ones included, latest start first.
func (a *App) adminProgrammes(w http.ResponseWriter, r *http.Request) {
	list, err := a.listProgrammes(r, `SELECT `+programmeColumns+` FROM programmes ORDER BY starts_on DESC, id DESC`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_programmes", &programmesPage{Programmes: list})
}

func (a *App) newProgrammeForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "programme_form", &programmeFormPage{})
}

func (a *App) editProgrammeForm(w http.ResponseWriter, r *http.Request) {
	p, ok := a.programmeFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "programme_form", &programmeFormPage{Programme: p})
}

func (a *App) createProgramme(w http.ResponseWriter, r *http.Request) {
	a.saveProgramme(w, r, programme{})
}

func (a *App) updateProgramme(w http.ResponseWriter, r *http.Request) {
	if p, ok := a.programmeFor(w, r); ok {
		a.saveProgramme(w, r, p)
	}
}

// saveProgramme reads the Programme form over existing (zero for a new one) and stores it.
// A new picture replaces the old one; nothing is stored unless every field and the picture are valid.
func (a *App) saveProgramme(w http.ResponseWriter, r *http.Request, existing programme) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := existing
	fail := func(status int, key string) {
		a.render(w, r, status, "programme_form", &programmeFormPage{pageBase{Error: text.T(lang, key)}, p})
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload)
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(http.StatusRequestEntityTooLarge, "adm.gallery.err.too-large")
			return
		}
		fail(http.StatusBadRequest, "adm.gallery.unread")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	p.Title = strings.TrimSpace(r.FormValue("title"))
	p.Blurb = strings.TrimSpace(r.FormValue("blurb"))
	p.Link = strings.TrimSpace(r.FormValue("link"))
	startsOn, dateErr := time.Parse(time.DateOnly, strings.TrimSpace(r.FormValue("starts_on")))
	p.StartsOn = startsOn
	switch {
	case p.Title == "":
		fail(http.StatusUnprocessableEntity, "programmes.err.title")
		return
	case dateErr != nil:
		fail(http.StatusUnprocessableEntity, "programmes.err.date")
		return
	case p.Link != "" && !isWebLink(p.Link):
		fail(http.StatusUnprocessableEntity, "plans.err.link")
		return
	}

	var newKey string
	file, _, err := r.FormFile("picture")
	if err == nil {
		defer file.Close()
		head := make([]byte, 512)
		n, err := io.ReadFull(file, head)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			a.serverError(w, r, err)
			return
		}
		mimeType, ok := sniffType(head[:n])
		if !ok || allowedTypes[mimeType] != "picture" {
			fail(http.StatusUnprocessableEntity, "programmes.err.picture")
			return
		}
		if newKey, _, err = a.store.Save(io.MultiReader(bytes.NewReader(head[:n]), file)); err != nil {
			a.serverError(w, r, err)
			return
		}
		p.PictureKey, p.pictureType = newKey, mimeType
	} else if !errors.Is(err, http.ErrMissingFile) {
		a.serverError(w, r, err)
		return
	}

	if p.ID == 0 {
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO programmes (title, starts_on, blurb, link, picture_key, picture_type)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, p.Title, p.StartsOn, p.Blurb, p.Link, p.PictureKey, p.pictureType).Scan(&p.ID)
	} else {
		_, err = a.db.ExecContext(r.Context(), `UPDATE programmes SET title = $2, starts_on = $3, blurb = $4, link = $5, picture_key = $6, picture_type = $7
			WHERE id = $1`, p.ID, p.Title, p.StartsOn, p.Blurb, p.Link, p.PictureKey, p.pictureType)
	}
	if err != nil {
		if newKey != "" {
			_ = a.store.Delete(r.Context(), newKey)
		}
		a.serverError(w, r, err)
		return
	}
	if newKey != "" && existing.PictureKey != "" {
		if err := a.store.Delete(r.Context(), existing.PictureKey); err != nil {
			a.logError(r, err)
		}
	}
	http.Redirect(w, r, "/admin/programmes", http.StatusSeeOther)
}

// deleteProgramme removes the row, then its picture.
func (a *App) deleteProgramme(w http.ResponseWriter, r *http.Request) {
	p, ok := a.programmeFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM programmes WHERE id = $1`, p.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.PictureKey != "" {
		if err := a.store.Delete(r.Context(), p.PictureKey); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/admin/programmes", http.StatusSeeOther)
}

// programmeFor loads the Programme named by the {id} path value. When there is none it writes
// the Not found page and returns false.
func (a *App) programmeFor(w http.ResponseWriter, r *http.Request) (programme, bool) {
	var p programme
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT `+programmeColumns+` FROM programmes WHERE id = $1`, id).
			Scan(&p.ID, &p.Title, &p.StartsOn, &p.Blurb, &p.Link, &p.PictureKey, &p.pictureType)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return p, false
		}
	}
	if err != nil {
		a.adminNotFound(w, r, "programmes.not-found.title", "programmes.not-found")
		return p, false
	}
	return p, true
}
