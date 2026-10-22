package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The products the Admin sells, as the prices table keys them, in the order the Prices page lists them.
const (
	caveTextProProduct = "cavetext-pro"
	labPlanProduct     = "lab-plan"
	proPlanProduct     = "pro-plan"
)

var products = []string{caveTextProProduct, labPlanProduct, proPlanProduct}

// price is one product's price, Normal price (0 when none) and payment link (empty when none).
type price struct {
	Product           string
	Sen, NormalSen    int
	PaymentLink       string
	PriceIn, NormalIn string // as the Prices form shows them, in RM without the RM, or as the Admin posted them
}

// maxPriceSen bounds a price, so a typo can't ask a Learner for a fortune.
const maxPriceSen = 9999999

// priceOf reads one product's price.
func (a *App) priceOf(ctx context.Context, product string) (price, error) {
	p := price{Product: product}
	err := a.db.QueryRowContext(ctx, `SELECT price_sen, COALESCE(normal_sen, 0), payment_link FROM prices WHERE product = $1`, product).
		Scan(&p.Sen, &p.NormalSen, &p.PaymentLink)
	if err != nil {
		return p, fmt.Errorf("read the %s price: %w", product, err)
	}
	return p, nil
}

type pricesPage struct {
	pageBase
	Prices []price
}

func (a *App) adminPrices(w http.ResponseWriter, r *http.Request) {
	p := &pricesPage{}
	for _, product := range products {
		pr, err := a.priceOf(r.Context(), product)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		pr.PriceIn = strings.TrimPrefix(rm(pr.Sen), "RM")
		if pr.NormalSen > 0 {
			pr.NormalIn = strings.TrimPrefix(rm(pr.NormalSen), "RM")
		}
		p.Prices = append(p.Prices, pr)
	}
	if r.URL.Query().Has("saved") {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Message = text.T(lang, "admin.common.saved")
	}
	a.render(w, r, http.StatusOK, "admin_prices", p)
}

// savePrices saves every product's price at once, or none when any is refused.
func (a *App) savePrices(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &pricesPage{}
	for _, product := range products {
		pr := price{Product: product,
			PriceIn:     strings.TrimSpace(r.PostFormValue(product + "_price")),
			NormalIn:    strings.TrimSpace(r.PostFormValue(product + "_normal")),
			PaymentLink: strings.TrimSpace(r.PostFormValue(product + "_link"))}
		var ok, normalOK bool
		pr.Sen, ok = parseRM(pr.PriceIn)
		pr.NormalSen, normalOK = parseRM(pr.NormalIn)
		msg := ""
		switch {
		case !ok || pr.Sen > maxPriceSen:
			msg = "prices.err.price"
		case pr.Sen == 0:
			msg = "prices.err.zero"
		case pr.NormalIn != "" && (!normalOK || pr.NormalSen > maxPriceSen):
			msg = "prices.err.normal"
		case pr.NormalIn != "" && pr.NormalSen <= pr.Sen:
			msg = "prices.err.normal-low"
		case pr.PaymentLink != "" && !isWebLink(pr.PaymentLink):
			msg = "prices.err.link"
		}
		if msg != "" && p.Error == "" {
			p.Error = text.T(lang, "prices."+product) + ": " + text.T(lang, msg)
		}
		p.Prices = append(p.Prices, pr)
	}
	if p.Error != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "admin_prices", p)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		for _, pr := range p.Prices {
			if _, err = tx.ExecContext(r.Context(), `UPDATE prices SET price_sen = $2, normal_sen = NULLIF($3, 0), payment_link = $4 WHERE product = $1`,
				pr.Product, pr.Sen, pr.NormalSen, pr.PaymentLink); err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		a.serverError(w, r, fmt.Errorf("save prices: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/prices?saved", http.StatusSeeOther)
}
