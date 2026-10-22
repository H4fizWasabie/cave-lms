package app

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// bclWebhookBody is the part of BCL's webhook CAVELMS reads. BCL signs nothing, so the secret key in the address is
// the only proof the call is BCL's.
type bclWebhookBody struct {
	Event string `json:"event"`
	Data  struct {
		FormURL string `json:"form_url"`
		Main    struct {
			PayerName  string          `json:"payer_name"`
			PayerEmail string          `json:"payer_email"`
			Order      string          `json:"order_number"`
			Subtotal   string          `json:"subtotal_amount"`
			Paid       json.RawMessage `json:"is_paid"`
		} `json:"main_data"`
		// Custom holds the form's own fields, keyed by a name BCL makes from each label. PHP sends [] when there are
		// none, so it stays raw until read.
		Custom json.RawMessage `json:"custom_fields_data"`
	} `json:"data"`
}

// paid reads is_paid, which BCL sends as 1; true and "1" are taken too.
func (b *bclWebhookBody) paid() bool {
	switch string(bytes.TrimSpace(b.Data.Main.Paid)) {
	case "1", "true", `"1"`:
		return true
	}
	return false
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

// sameForm says whether a webhook's form address is the Pro payment link, ignoring case and a trailing slash.
func sameForm(formURL, link string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return link != "" && norm(formURL) == norm(link)
}

// bclWebhook takes BCL's webhooks. A paid order on the form the CaveText Pro payment link points to gives a year of
// Pro to the Learner named in the form's CAVELMS email field, or failing that the payer's email, creating their
// CaveText account if they have not signed in yet. Each order counts once. An order naming no Learner, or paying less
// than the price, is kept for an Admin. Everything else is answered 200 and only noted in the System log, so BCL
// does not retry it; a wrong or unset key gets the not-found page.
func (a *App) bclWebhook(w http.ResponseWriter, r *http.Request) {
	if a.bclWebhookKey == "" || subtle.ConstantTimeCompare([]byte(r.PathValue("key")), []byte(a.bclWebhookKey)) != 1 {
		a.notFound(w, r)
		return
	}
	note := func(format string, args ...any) {
		a.writeLog("info", "BCL webhook: "+fmt.Sprintf(format, args...), r.Method, "/cavetext/bcl/…")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	var b bclWebhookBody
	if err == nil {
		err = json.Unmarshal(raw, &b)
	}
	if err != nil {
		note("unreadable body: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	order := strings.TrimSpace(b.Data.Main.Order)
	var priceSen int
	var link string
	if err := a.db.QueryRowContext(r.Context(), `SELECT pro_price_sen, pro_payment_link FROM cavetext_settings WHERE id = 1`).Scan(&priceSen, &link); err != nil {
		a.serverError(w, r, fmt.Errorf("BCL webhook: read CaveText settings: %w", err))
		return
	}
	switch {
	case b.Event != "payment-success":
		note("order %s, event %q ignored", order, b.Event)
	case !sameForm(b.Data.FormURL, link):
		note("order %s is for %s, not the CaveText Pro payment link; ignored", order, b.Data.FormURL)
	case !b.paid():
		note("order %s is not paid; ignored", order)
	case order == "":
		note("paid order with no order number; ignored")
	default:
		outcome, err := a.recordProPayment(r.Context(), &b, order, priceSen)
		if err != nil {
			a.serverError(w, r, fmt.Errorf("BCL webhook: order %s: %w", order, err))
			return
		}
		note("order %s: %s", order, outcome)
	}
	w.WriteHeader(http.StatusOK)
}

// recordProPayment keeps a paid Pro order and gives its Learner a year of Pro, in one transaction. It says what
// happened, for the System log.
func (a *App) recordProPayment(ctx context.Context, b *bclWebhookBody, order string, priceSen int) (string, error) {
	subtotal, ok := parseRM(strings.TrimSpace(b.Data.Main.Subtotal))
	if !ok {
		subtotal = 0
	}
	cavelms, payer := b.cavelmsEmail(), strings.TrimSpace(b.Data.Main.PayerEmail)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO cavetext_payments (order_number, payer_name, payer_email, cavelms_email, subtotal_sen)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, order, strings.TrimSpace(b.Data.Main.PayerName), payer, cavelms, subtotal)
	if err != nil {
		return "", fmt.Errorf("keep payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "already received", tx.Commit()
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
	switch {
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("find Learner: %w", err)
	case learnerID == 0:
		return "names no Learner; kept for an Admin", tx.Commit()
	case subtotal < priceSen:
		if _, err := tx.ExecContext(ctx, `UPDATE cavetext_payments SET learner_id = $2 WHERE order_number = $1`, order, learnerID); err != nil {
			return "", err
		}
		return fmt.Sprintf("paid %s, under the %s price; kept for an Admin", rm(subtotal), rm(priceSen)), tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cavetext_accounts (learner_id) SELECT $1
		WHERE NOT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1) ON CONFLICT DO NOTHING`, learnerID); err != nil {
		return "", fmt.Errorf("create CaveText account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, giveProQuery, learnerID); err != nil {
		return "", fmt.Errorf("give Pro: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cavetext_payments SET learner_id = $2, granted = true WHERE order_number = $1`, order, learnerID); err != nil {
		return "", err
	}
	return fmt.Sprintf("gave Learner %d a year of Pro", learnerID), tx.Commit()
}

// unmatchedPayment is a Pro payment that gave no Pro, waiting on the Admin CaveText Learners page.
type unmatchedPayment struct {
	Order, PayerName, PayerEmail, CAVELMSEmail, Paid string
	Received                                         time.Time
}

// When is when the payment arrived, as the page shows it.
func (p unmatchedPayment) When() string { return p.Received.UTC().Format(periodTime) }

func (a *App) unmatchedPayments(ctx context.Context) ([]unmatchedPayment, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT order_number, received_at, payer_name, payer_email, cavelms_email, subtotal_sen
		FROM cavetext_payments WHERE NOT granted AND done_at IS NULL ORDER BY received_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ps []unmatchedPayment
	for rows.Next() {
		var p unmatchedPayment
		var sen int
		if err := rows.Scan(&p.Order, &p.Received, &p.PayerName, &p.PayerEmail, &p.CAVELMSEmail, &sen); err != nil {
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
		WHERE order_number = $1 AND NOT granted AND done_at IS NULL`, r.PathValue("order")); err != nil {
		a.serverError(w, r, fmt.Errorf("mark payment done: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext", http.StatusSeeOther)
}
