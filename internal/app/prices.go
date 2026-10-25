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

// offer is one product as a Learner page offers it: its price, its Normal price crossed out when set, and Buy,
// which opens its payment link or, with none, asks the Learner to contact us.
type offer struct {
	Product, Price, Normal, PaymentLink string // Price and Normal in RM: RM25; Normal empty when none
	Promo                               string // the lowest running Promo price in RM, below Price; empty when none
	Renew                               bool   // Buy reads Renew: the Learner already has the product
}

// offerBenefits are the catalog keys of what each product includes, listed under its price.
var offerBenefits = map[string][]string{
	caveTextProProduct: {"offers.cavetext-pro.unlimited", "offers.cavetext-pro.user", "offers.cavetext-pro.ready", "offers.cavetext-pro.keep"},
	labPlanProduct:     {"offers.lab-plan.build", "offers.lab-plan.deploy", "offers.lab-plan.hosting", "offers.lab-plan.code"},
	proPlanProduct:     {"offers.pro-plan.cavetext", "offers.pro-plan.lab", "offers.pro-plan.ebooks", "offers.pro-plan.deploy", "offers.pro-plan.storage", "offers.pro-plan.both"},
}

func (o offer) Benefits() []string { return offerBenefits[o.Product] }

// offersOf reads the products' prices as offers, in the order given.
func (a *App) offersOf(ctx context.Context, products ...string) ([]offer, error) {
	var offers []offer
	for _, product := range products {
		p, err := a.priceOf(ctx, product)
		if err != nil {
			return nil, err
		}
		o := offer{Product: product, Price: rm(p.Sen), PaymentLink: p.PaymentLink}
		promoSen, err := lowestPromoSen(ctx, a.db, product)
		if err != nil {
			return nil, err
		}
		if promoSen > 0 && promoSen < p.Sen {
			o.Promo = rm(promoSen)
		}
		if p.NormalSen > 0 {
			o.Normal = rm(p.NormalSen)
		}
		offers = append(offers, o)
	}
	return offers, nil
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
	links := map[string]bool{} // the BCL webhook gives the first product whose link matches, so no two may share one
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
		case pr.PaymentLink != "" && links[formKey(pr.PaymentLink)]:
			msg = "prices.err.link-shared"
		}
		links[formKey(pr.PaymentLink)] = true
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

// pricing is the public Pricing page: the three products the Admin sells, as offers.
func (a *App) pricing(w http.ResponseWriter, r *http.Request) {
	offers, err := a.offersOf(r.Context(), products...)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if learner != nil { // CaveText Pro running: its Buy reads Renew, as on /cavetext
		var pro bool
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM cavetext_pro_periods pp
			WHERE pp.learner_id = $1 AND `+activePeriodSQL+`)`, learner.ID).Scan(&pro); err != nil {
			a.serverError(w, r, err)
			return
		}
		for i := range offers {
			offers[i].Renew = pro && offers[i].Product == caveTextProProduct
		}
	}
	a.render(w, r, http.StatusOK, "pricing", &struct {
		pageBase
		Offers []offer
	}{pageBase: pageBase{Learner: learner}, Offers: offers})
}
