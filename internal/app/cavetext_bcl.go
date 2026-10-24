package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

// bclWebhookBody is the part of BCL's webhook CAVELMS reads. BCL signs nothing, so the secret key in the address
// proves the call is BCL's, and with a BCL API token each order is also confirmed with BCL (confirmBCLOrder).
type bclWebhookBody struct {
	Event string `json:"event"`
	Data  struct {
		FormURL string `json:"form_url"`
		Main    struct {
			PayerName  bclText `json:"payer_name"`
			PayerEmail bclText `json:"payer_email"`
			Order      bclText `json:"order_number"`
			Kind       bclText `json:"transaction_type"` // "test" for Test Mode
			Subtotal   bclText `json:"subtotal_amount"`
			Amount     bclText `json:"amount"`          // what the payer was charged, fee included
			Discount   bclText `json:"discount_amount"` // a coupon's discount, off the subtotal; empty with none
			Paid       bclText `json:"is_paid"`
		} `json:"main_data"`
		// Custom holds the form's own fields, keyed by a name BCL makes from each label. PHP sends [] when there are
		// none, so it stays raw until read.
		Custom json.RawMessage `json:"custom_fields_data"`
	} `json:"data"`
}

// couponFields lists main_data's coupon and discount fields as ", name=value", sorted, so a Test Mode payment shows
// how BCL reports a coupon. Empty when there are none.
func couponFields(raw []byte) string {
	var body struct {
		Data struct {
			Main map[string]bclText `json:"main_data"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	var fields []string
	for k, v := range body.Data.Main {
		if l := strings.ToLower(k); strings.Contains(l, "coupon") || strings.Contains(l, "discount") {
			fields = append(fields, ", "+k+"="+v.String())
		}
	}
	slices.Sort(fields)
	return strings.Join(fields, "")
}

// bclText is a value BCL may send as a string, a number or a bool; it is kept as text, trimmed. null is empty.
type bclText string

func (t *bclText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) != nil {
		s = string(b) // a number or bool, as written
		if s == "null" {
			s = ""
		}
	}
	*t = bclText(strings.TrimSpace(s))
	return nil
}

func (t bclText) String() string { return string(t) }

// paidSen is what the order paid before BCL's fee: the subtotal less any coupon's discount, which BCL sends apart.
// readable is false when either amount is not a number of RM.
func (b *bclWebhookBody) paidSen() (sen int, readable bool) {
	sen, readable = parseRM(b.Data.Main.Subtotal.String())
	if d := b.Data.Main.Discount.String(); readable && d != "" {
		var discount int
		discount, readable = parseRM(d)
		sen -= discount
	}
	return sen, readable && sen >= 0
}

// paid reads is_paid, which BCL sends as 1; true is taken too.
func (b *bclWebhookBody) paid() bool {
	return b.Data.Main.Paid == "1" || b.Data.Main.Paid == "true"
}

// cavelmsEmail is the form's CAVELMS account email field: the first custom field whose name mentions CAVELMS and
// whose value looks like an email address.
func (b *bclWebhookBody) cavelmsEmail() string {
	var fields map[string]any
	if json.Unmarshal(b.Data.Custom, &fields) != nil {
		return ""
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v, ok := fields[name].(string); ok && strings.Contains(strings.ToLower(name), "cavelms") && strings.Contains(v, "@") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// formKey is a form address with case and a trailing slash ignored, as BCL may send either.
func formKey(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }

// sameForm says whether a webhook's form address is a payment link, ignoring case and a trailing slash.
func sameForm(formURL, link string) bool { return link != "" && formKey(formURL) == formKey(link) }

// bclWebhook takes BCL's webhooks. A paid, live order on the form a product's payment link points to gives that
// product (see recordPayment) to the Learner named in the form's CAVELMS email field, or failing that the payer's email, creating
// their CaveText account if they have not signed in yet. Each order counts once. A paid, live order that names no
// Learner, pays less than the price or comes from another form is kept for an Admin. Test Mode payments, unpaid
// orders and other events are answered 200 and only noted in the System log, so BCL does not retry them. A database
// failure is answered 500, so BCL tries again. With a BCL API token, each paid order is first confirmed with BCL
// (confirmBCLOrder): one BCL does not back is answered 200 and only logged as an error, and BCL out of reach is
// answered 500. With BCL_PAYMENTS=test (staging, ADR 0010) Test Mode orders count and live ones are only noted. A
// wrong or unset key gets the not-found page. Nothing logged names the key or the token: every line gives the path as
// /cavetext/bcl/….
func (a *App) bclWebhook(w http.ResponseWriter, r *http.Request) {
	if a.bclWebhookKey == "" || subtle.ConstantTimeCompare([]byte(r.PathValue("key")), []byte(a.bclWebhookKey)) != 1 {
		a.notFound(w, r)
		return
	}
	note := func(level, format string, args ...any) {
		a.writeLog(level, "BCL webhook: "+fmt.Sprintf(format, args...), r.Method, "/cavetext/bcl/…")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	var b bclWebhookBody
	if err == nil {
		err = json.Unmarshal(raw, &b)
	}
	if err != nil {
		// The body is kept so a paid order BCL sent in an unexpected shape can still be found and given by hand.
		note("error", "unreadable body (%v): %.4000s", err, raw)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	order := b.Data.Main.Order.String()
	var form price // the product whose payment link is the paid form; none when Product is empty
	for _, product := range products {
		p, err := a.priceOf(r.Context(), product)
		if err != nil {
			note("error", "order %s: %v", order, err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if sameForm(b.Data.FormURL, p.PaymentLink) {
			form = p
			break
		}
	}
	test := b.Data.Main.Kind == "test"
	switch {
	case b.Event != "payment-success":
		note("info", "order %s, event %q ignored", order, b.Event)
	case !b.paid():
		note("info", "order %s is not paid; ignored", order)
	case !test && a.bclTestPayments:
		note("info", "order %s is a live payment on %s and this site counts only Test Mode (BCL_PAYMENTS=test); nothing given",
			order, b.Data.FormURL)
	case test && !a.bclTestPayments:
		note("info", "order %s is a Test Mode payment on %s, subtotal %s, amount %s%s; nothing given",
			order, b.Data.FormURL, b.Data.Main.Subtotal, b.Data.Main.Amount, couponFields(raw))
	case order == "":
		note("info", "paid order with no order number on %s; ignored", b.Data.FormURL)
	default:
		refused, err := a.confirmBCLOrder(r.Context(), &b, order)
		if err != nil {
			note("error", "order %s: confirm with BCL: %v", order, err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if refused != "" {
			note("error", "order %s %s; nothing given", order, refused)
			break
		}
		outcome, err := a.recordPayment(r.Context(), &b, order, form)
		if err != nil {
			note("error", "order %s: %v", order, err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		note("info", "order %s: %s", order, outcome)
	}
	w.WriteHeader(http.StatusOK)
}

var bclClient = &http.Client{Timeout: 10 * time.Second}

// confirmBCLOrder looks order up in BCL's API, with a.bclAPIToken; with none set it confirms every order. BCL backs
// the webhook b when it knows the order as successful, for the same amount and payer email, and that amount covers
// the subtotal less discount the price is checked against, so a cheap real order cannot claim a dearer form. refused
// says why not, for the System log. err is BCL out of reach or failing, so BCL should retry.
func (a *App) confirmBCLOrder(ctx context.Context, b *bclWebhookBody, order string) (refused string, err error) {
	if a.bclAPIToken == "" {
		return "", nil
	}
	lookup := "/transactions/" // finds live orders only
	if a.bclTestPayments {
		lookup = "/transaction/" // finds Test Mode orders too
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.bclAPIURL+lookup+url.PathEscape(order), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.bclAPIToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cavelms/1.0") // BCL blocks Go's default agent
	res, err := bclClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "is not one BCL knows", nil
	default:
		return "", fmt.Errorf("BCL answered %s", res.Status)
	}
	var found struct {
		Data struct {
			Transaction struct {
				Amount     bclText `json:"amount"`
				PayerEmail bclText `json:"payer_email"`
				Status     bclText `json:"status"`
			} `json:"transaction"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&found); err != nil {
		return "", fmt.Errorf("read BCL's answer: %w", err)
	}
	t, m := found.Data.Transaction, b.Data.Main
	charged, readable := parseRM(t.Amount.String())
	claimed, claimedReadable := parseRM(m.Amount.String())
	paid, _ := b.paidSen()
	switch {
	case t.Status != "3": // 3 is successful, 2 failed
		return fmt.Sprintf("has status %s at BCL, not 3 (successful)", t.Status), nil
	case !readable || !claimedReadable || charged != claimed:
		return fmt.Sprintf("is for %s at BCL, not %s", t.Amount, m.Amount), nil
	case !strings.EqualFold(t.PayerEmail.String(), m.PayerEmail.String()):
		return fmt.Sprintf("was paid by %s at BCL, not %s", t.PayerEmail, m.PayerEmail), nil
	case charged < paid:
		return fmt.Sprintf("charged %s at BCL, under the %s the webhook says it paid before the fee", t.Amount, rm(paid)), nil
	}
	return "", nil
}

// recordPayment keeps a paid order on the form of product (none when form.Product is empty) and, when it names a
// Learner and pays at least the product's price, gives that Learner the product, in one transaction. It says what
// happened, for the System log.
func (a *App) recordPayment(ctx context.Context, b *bclWebhookBody, order string, form price) (string, error) {
	paid, readable := b.paidSen() // kept in subtotal_sen: what was paid, less any discount
	cavelms, payer := b.cavelmsEmail(), b.Data.Main.PayerEmail.String()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO cavetext_payments (order_number, payer_name, payer_email, cavelms_email, subtotal_sen, product)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')) ON CONFLICT DO NOTHING`, order, b.Data.Main.PayerName.String(), payer, cavelms, paid, form.Product)
	if err != nil {
		return "", fmt.Errorf("keep payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "already received", tx.Commit()
	}
	if form.Product == "" {
		return fmt.Sprintf("paid on %s, no current payment link; kept for an Admin", b.Data.FormURL), tx.Commit()
	}
	var learnerID int64
	for _, email := range []string{cavelms, payer} {
		if email == "" {
			continue
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM learners WHERE lower(email) = lower($1)`, email).Scan(&learnerID)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			break
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find Learner: %w", err)
	}
	enough := form.Sen // the price, or the lowest running Promo's when that is lower
	if promoSen, err := lowestPromoSen(ctx, tx, form.Product); err != nil {
		return "", fmt.Errorf("read Promos: %w", err)
	} else if promoSen > 0 && promoSen < enough {
		enough = promoSen
	}
	switch {
	case learnerID == 0:
		return "names no Learner; kept for an Admin", tx.Commit()
	case !readable || paid < enough:
		if err := markPayment(ctx, tx, order, learnerID, false); err != nil {
			return "", err
		}
		if !readable {
			return fmt.Sprintf("unreadable amount %q less discount %q; kept for an Admin", b.Data.Main.Subtotal, b.Data.Main.Discount), tx.Commit()
		}
		return fmt.Sprintf("paid %s, under the %s price; kept for an Admin", rm(paid), rm(enough)), tx.Commit()
	}
	if form.Product != labPlanProduct {
		if err := ensureCaveTextAccount(ctx, tx, learnerID); err != nil {
			return "", err
		}
	}
	planned := true // false when no Plan is marked as the Lab Plan, so a Lab Plan or Pro Plan gave no Plan period
	switch form.Product {
	case caveTextProProduct:
		err = givePro(ctx, tx, learnerID)
	case labPlanProduct:
		planned, err = giveLabPlan(ctx, tx, learnerID)
	case proPlanProduct:
		planned, err = giveProPlan(ctx, tx, learnerID)
	}
	if err != nil {
		return "", err
	}
	granted := planned || form.Product != labPlanProduct // a Pro Plan still gave CaveText Pro
	if err := markPayment(ctx, tx, order, learnerID, granted); err != nil {
		return "", err
	}
	if !granted {
		return "no Plan is marked as the Lab Plan; kept for an Admin", tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if !planned {
		a.warnNoLabPlan(learnerID)
	}
	return fmt.Sprintf("gave Learner %d %s", learnerID, form.Product), nil
}

// unmatchedPayment is a Payment that gave nothing, waiting on the Admin CaveText Learners page.
type unmatchedPayment struct {
	Order, PayerName, PayerEmail, CAVELMSEmail, Paid string
	Product                                          string // whose form it was paid on, as prices keys it; empty for none
	Received                                         time.Time
}

// When is when the payment arrived, as the page shows it.
func (p unmatchedPayment) When() string { return p.Received.UTC().Format(periodTime) }

// unmatchedPayments lists the Payments that gave nothing and no Admin has marked done, oldest first.
func (a *App) unmatchedPayments(ctx context.Context) ([]unmatchedPayment, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT order_number, received_at, payer_name, payer_email, cavelms_email, subtotal_sen, COALESCE(product, '')
		FROM cavetext_payments WHERE NOT granted AND done_at IS NULL ORDER BY received_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ps []unmatchedPayment
	for rows.Next() {
		var p unmatchedPayment
		var sen int
		if err := rows.Scan(&p.Order, &p.Received, &p.PayerName, &p.PayerEmail, &p.CAVELMSEmail, &sen, &p.Product); err != nil {
			return nil, err
		}
		p.Paid = rm(sen)
		ps = append(ps, p)
	}
	return ps, rows.Err()
}

// paymentDone takes an unmatched payment off the Admin page, once an Admin has sorted it out by hand.
func (a *App) paymentDone(w http.ResponseWriter, r *http.Request) {
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_payments SET done_at = now()
		WHERE order_number = $1 AND NOT granted AND done_at IS NULL`, r.PostFormValue("order")); err != nil {
		a.serverError(w, r, fmt.Errorf("mark payment done: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext", http.StatusSeeOther)
}

// markPayment names the Learner the payment is for and whether it gave them the product.
func markPayment(ctx context.Context, db execer, order string, learnerID int64, granted bool) error {
	if _, err := db.ExecContext(ctx, `UPDATE cavetext_payments SET learner_id = $2, granted = $3 WHERE order_number = $1`, order, learnerID, granted); err != nil {
		return fmt.Errorf("mark payment's Learner: %w", err)
	}
	return nil
}
