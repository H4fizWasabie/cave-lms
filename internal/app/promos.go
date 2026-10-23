package app

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// runningPromoSQL is the one definition of a running Promo: started, not past its end date in Malaysia, and not
// ended early.
const runningPromoSQL = `starts_on <= (now() AT TIME ZONE 'Asia/Kuala_Lumpur')::date
	AND ends_on >= (now() AT TIME ZONE 'Asia/Kuala_Lumpur')::date AND ended_at IS NULL`

// promo is a Promo as the Promos page lists it, or as its form holds it while being corrected.
type promo struct {
	ID               int64
	Code, Product    string
	Sen              int
	StartsOn, EndsOn time.Time
	Running, Ended   bool   // Ended: ended early by an Admin
	Upcoming         bool   // not started yet
	PriceIn          string // the form's price as typed, kept when refused
	StartsIn, EndsIn string
}

func (p promo) Price() string             { return rm(p.Sen) }
func (p promo) Starts(lang string) string { return text.Date(lang, p.StartsOn) }
func (p promo) Ends(lang string) string   { return text.Date(lang, p.EndsOn) }

type promosPage struct {
	pageBase
	Promos   []promo
	Form     promo
	Products []string
}

// lowestPromoSen is the lowest running Promo price for product, or 0 when none runs.
func lowestPromoSen(ctx context.Context, q rowQueryer, product string) (int, error) {
	var sen int
	err := q.QueryRowContext(ctx, `SELECT COALESCE(min(price_sen), 0) FROM promos WHERE product = $1 AND `+runningPromoSQL, product).Scan(&sen)
	return sen, err
}

// adminPromos lists every Promo, running and past, latest start first, above the form that adds one.
func (a *App) adminPromos(w http.ResponseWriter, r *http.Request) {
	a.renderPromos(w, r, http.StatusOK, &promosPage{})
}

func (a *App) renderPromos(w http.ResponseWriter, r *http.Request, status int, p *promosPage) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id, code, product, price_sen, starts_on, ends_on, `+runningPromoSQL+`, ended_at IS NOT NULL,
		ended_at IS NULL AND starts_on > (now() AT TIME ZONE 'Asia/Kuala_Lumpur')::date
		FROM promos ORDER BY starts_on DESC, id DESC`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pr promo
		if err := rows.Scan(&pr.ID, &pr.Code, &pr.Product, &pr.Sen, &pr.StartsOn, &pr.EndsOn, &pr.Running, &pr.Ended, &pr.Upcoming); err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Promos = append(p.Promos, pr)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Products = products
	a.render(w, r, status, "admin_promos", p)
}

// addPromo adds a Promo from the form, or shows the form again with what is wrong.
func (a *App) addPromo(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := promo{Code: strings.TrimSpace(r.PostFormValue("code")), Product: r.PostFormValue("product"),
		PriceIn:  strings.TrimSpace(r.PostFormValue("price")),
		StartsIn: strings.TrimSpace(r.PostFormValue("starts_on")), EndsIn: strings.TrimSpace(r.PostFormValue("ends_on"))}
	sen, priceOK := parseRM(f.PriceIn)
	starts, startsErr := time.Parse(time.DateOnly, f.StartsIn)
	ends, endsErr := time.Parse(time.DateOnly, f.EndsIn)
	msg := ""
	switch {
	case f.Code == "" || strings.ContainsAny(f.Code, " \t"):
		msg = "promos.err.code"
	case !slices.Contains(products, f.Product):
		msg = "promos.err.product"
	case !priceOK || sen == 0 || sen > maxPriceSen:
		msg = "promos.err.price"
	case startsErr != nil || endsErr != nil:
		msg = "promos.err.dates"
	case ends.Before(starts):
		msg = "promos.err.ends"
	}
	if msg != "" {
		a.renderPromos(w, r, http.StatusUnprocessableEntity, &promosPage{pageBase: pageBase{Error: text.T(lang, msg)}, Form: f})
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `INSERT INTO promos (code, product, price_sen, starts_on, ends_on) VALUES ($1, $2, $3, $4, $5)`,
		f.Code, f.Product, sen, starts, ends); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/promos", http.StatusSeeOther)
}

// endPromo ends a running Promo now; it stays listed.
func (a *App) endPromo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE promos SET ended_at = now() WHERE id = $1 AND ended_at IS NULL`, id); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/promos", http.StatusSeeOther)
}
