package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// organisation is an Organisation a Live Course is run for. LogoKey names its logo, a PNG or JPEG, in storage.
type organisation struct {
	ID      int64
	Name    string
	LogoKey string
}

// logoSearchBase is a web image search, before its query.
const logoSearchBase = "https://www.google.com/search?tbm=isch&q="

// LogoSearch is a web image search for the Organisation's name, which the Admin opens to find its logo.
func (o organisation) LogoSearch() string {
	return logoSearchBase + url.QueryEscape(o.Name)
}

// LogoSearchBase is logoSearchBase, for logo-search.js to search for the name as it is typed.
func (organisation) LogoSearchBase() string { return logoSearchBase }

type organisationsPage struct {
	pageBase
	Organisations []organisation
}

type organisationFormPage struct {
	pageBase
	Organisation organisation
}

// maxOrganisationLogo caps the logo; the form may be a little larger, for its other fields.
const maxOrganisationLogo = 2 << 20

// logoSide is the longest side, in pixels, an SVG logo is drawn at: sharp at any size it prints on a Certificate.
const logoSide = 1200

// svgScript finds what can run in an SVG: a script element, an on... event attribute or a javascript: link.
var svgScript = regexp.MustCompile(`(?i)<script|\son[a-z]+\s*=|javascript:`)

// organisations lists every Organisation, by name, for the Admin and for the Live Course form.
func (a *App) organisations(ctx context.Context) ([]organisation, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, logo_key FROM organisations ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []organisation
	for rows.Next() {
		var o organisation
		if err := rows.Scan(&o.ID, &o.Name, &o.LogoKey); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (a *App) adminOrganisations(w http.ResponseWriter, r *http.Request) {
	list, err := a.organisations(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_organisations", &organisationsPage{Organisations: list})
}

func (a *App) newOrganisationForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "organisation_form", &organisationFormPage{})
}

func (a *App) editOrganisationForm(w http.ResponseWriter, r *http.Request) {
	if o, ok := a.organisationFor(w, r); ok {
		a.render(w, r, http.StatusOK, "organisation_form", &organisationFormPage{Organisation: o})
	}
}

func (a *App) createOrganisation(w http.ResponseWriter, r *http.Request) {
	a.saveOrganisation(w, r, organisation{})
}

func (a *App) updateOrganisation(w http.ResponseWriter, r *http.Request) {
	if o, ok := a.organisationFor(w, r); ok {
		a.saveOrganisation(w, r, o)
	}
}

// saveOrganisation reads the Organisation form over existing (zero for a new one) and stores it. A new one needs a
// logo. A new logo is stored as a new file and the old one kept, as Certificates issued with it still print it.
func (a *App) saveOrganisation(w http.ResponseWriter, r *http.Request, existing organisation) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	o := existing
	fail := func(status int, key string) {
		a.render(w, r, status, "organisation_form", &organisationFormPage{pageBase{Error: text.T(lang, key)}, o})
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOrganisationLogo+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(http.StatusRequestEntityTooLarge, "orgs.err.too-large")
			return
		}
		fail(http.StatusBadRequest, "orgs.err.unread")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	o.Name = strings.TrimSpace(r.FormValue("name"))
	if o.Name == "" {
		fail(http.StatusUnprocessableEntity, "orgs.err.name")
		return
	}
	var logo []byte
	file, header, err := r.FormFile("logo")
	if err == nil && header.Size > maxOrganisationLogo {
		file.Close()
		fail(http.StatusRequestEntityTooLarge, "orgs.err.too-large")
		return
	}
	if err == nil {
		logo, err = io.ReadAll(file)
		file.Close()
	}
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		a.serverError(w, r, err)
		return
	}
	if logo != nil || o.ID == 0 {
		if logo, err = logoImage(logo); err != nil {
			fail(http.StatusUnprocessableEntity, "orgs.err.logo")
			return
		}
		if o.LogoKey, _, err = a.store.Save(bytes.NewReader(logo)); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if o.ID == 0 {
		_, err = a.db.ExecContext(r.Context(), `INSERT INTO organisations (name, logo_key) VALUES ($1, $2)`, o.Name, o.LogoKey)
	} else {
		_, err = a.db.ExecContext(r.Context(), `UPDATE organisations SET name = $2, logo_key = $3 WHERE id = $1`, o.ID, o.Name, o.LogoKey)
	}
	if err != nil {
		if o.LogoKey != existing.LogoKey {
			_ = a.store.Delete(r.Context(), o.LogoKey)
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/organisations", http.StatusSeeOther)
}

// logoImage returns the logo to store: a PNG or JPEG as uploaded, or an SVG drawn as a PNG, so no SVG is ever
// stored or served. It fails on anything else, and on an SVG that could run a script.
func logoImage(data []byte) ([]byte, error) {
	if ct := http.DetectContentType(data); ct == "image/png" || ct == "image/jpeg" {
		return data, nil
	}
	if !isSVG(data) || svgScript.Match(data) {
		return nil, errors.New("not a PNG, JPEG or script-free SVG")
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	vw, vh := icon.ViewBox.W, icon.ViewBox.H
	if vw <= 0 || vh <= 0 {
		return nil, errors.New("SVG has no size")
	}
	scale := logoSide / max(vw, vh)
	w, h := max(int(math.Round(vw*scale)), 1), max(int(math.Round(vh*scale)), 1)
	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h)) // transparent until drawn on
	icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, img, img.Bounds())), 1)
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	return buf.Bytes(), err
}

// isSVG reports whether data is XML whose root element is svg.
func isSVG(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		t, err := d.Token()
		if err != nil {
			return false
		}
		if el, ok := t.(xml.StartElement); ok {
			return el.Name.Local == "svg"
		}
	}
}

// organisationLogo serves an Organisation's logo to the Admin. Only PNG and JPEG are ever stored.
func (a *App) organisationLogo(w http.ResponseWriter, r *http.Request) {
	o, ok := a.organisationFor(w, r)
	if !ok {
		return
	}
	a.serveImage(w, r, o.LogoKey)
}

// deleteOrganisationPage is the confirmation page for deleting o, refused with the catalog key errKey when not "".
func deleteOrganisationPage(o organisation, errKey string) confirmation {
	return confirmation{pageBase: pageBase{Error: errKey}, Side: "organisations", Heading: "orgs.delete", Question: "adm.delete-ask",
		Cancel: "/admin/organisations/" + strconv.FormatInt(o.ID, 10) + "/edit", Action: "/admin/organisations/" + strconv.FormatInt(o.ID, 10) + "/delete"}
}

// confirmDeleteOrganisation asks the Admin to confirm deleting an Organisation.
func (a *App) confirmDeleteOrganisation(w http.ResponseWriter, r *http.Request) {
	if o, ok := a.organisationFor(w, r); ok {
		a.confirm(w, r, http.StatusOK, deleteOrganisationPage(o, ""), o.Name)
	}
}

// deleteOrganisation removes an Organisation no Live Course uses, once its confirmation page is posted. Its logo
// file is kept for the Certificates that print it.
func (a *App) deleteOrganisation(w http.ResponseWriter, r *http.Request) {
	o, ok := a.organisationFor(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM organisations WHERE id = $1`, o.ID)
	if isForeignKeyError(err) {
		a.confirm(w, r, http.StatusConflict, deleteOrganisationPage(o, "orgs.err.in-use"), o.Name)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/organisations", http.StatusSeeOther)
}

// organisationFor loads the Organisation named by the {id} path value. When there is none it writes
// the Not found page and returns false.
func (a *App) organisationFor(w http.ResponseWriter, r *http.Request) (organisation, bool) {
	var o organisation
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT id, name, logo_key FROM organisations WHERE id = $1`, id).Scan(&o.ID, &o.Name, &o.LogoKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return o, false
		}
	}
	if err != nil {
		a.adminNotFound(w, r, "orgs.not-found.title", "orgs.not-found")
		return o, false
	}
	return o, true
}
