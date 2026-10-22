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
			PayerName  bclText `json:"payer_name"`
			PayerEmail bclText `json:"payer_email"`
			Order      bclText `json:"order_number"`
			Kind       bclText `json:"transaction_type"` // "test" for Test Mode
			Subtotal   bclText `json:"subtotal_amount"`
			Paid       bclText `json:"is_paid"`
		} `json:"main_data"`
		// Custom holds the form's own fields, keyed by a name BCL makes from each label. PHP sends [] when there are
		// none, so it stays raw until read.
		Custom json.RawMessage `json:"custom_fields_data"`
	} `json:"data"`
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

// sameForm says whether a webhook's form address is the Pro payment link, ignoring case and a trailing slash.
func sameForm(formURL, link string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return link != "" && norm(formURL) == norm(link)
}

// bclWebhook takes BCL's webhooks. A paid, live order on the form the CaveText Pro payment link points to gives a
// year of Pro to the Learner named in the form's CAVELMS email field, or failing that the payer's email, creating
// their CaveText account if they have not signed in yet. Each order counts once. A paid, live order that names no
// Learner, pays less than the price or comes from another form is kept for an Admin. Test Mode payments, unpaid
// orders and other events are answered 200 and only noted in the System log, so BCL does not retry them. A database
// failure is answered 500, so BCL tries again. A wrong or unset key gets the not-found page. Nothing logged names the
// key: every line gives the path as /cavetext/bcl/….
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
	var priceSen int
	var link string
	if err := a.db.QueryRowContext(r.Context(), `SELECT pro_price_sen, pro_payment_link FROM cavetext_settings WHERE id = 1`).Scan(&priceSen, &link); err != nil {
		note("error", "order %s: read CaveText settings: %v", order, err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	switch {
	case b.Event != "payment-success":
		note("info", "order %s, event %q ignored", order, b.Event)
	case !b.paid():
		note("info", "order %s is not paid; ignored", order)
	case b.Data.Main.Kind == "test":
		note("info", "order %s is a Test Mode payment on %s; no Pro given", order, b.Data.FormURL)
	case order == "":
		note("info", "paid order with no order number on %s; ignored", b.Data.FormURL)
	default:
		outcome, err := a.recordProPayment(r.Context(), &b, order, priceSen, sameForm(b.Data.FormURL, link))
		if err != nil {
			note("error", "order %s: %v", order, err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		note("info", "order %s: %s", order, outcome)
	}
	w.WriteHeader(http.StatusOK)
}

// recordProPayment keeps a paid order and, when it is on the Pro form, gives its Learner a year of Pro, in one
// transaction. It says what happened, for the System log.
func (a *App) recordProPayment(ctx context.Context, b *bclWebhookBody, order string, priceSen int, onProForm bool) (string, error) {
	subtotal, readable := parseRM(b.Data.Main.Subtotal.String())
	cavelms, payer := b.cavelmsEmail(), b.Data.Main.PayerEmail.String()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO cavetext_payments (order_number, payer_name, payer_email, cavelms_email, subtotal_sen)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, order, b.Data.Main.PayerName.String(), payer, cavelms, subtotal)
	if err != nil {
		return "", fmt.Errorf("keep payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "already received", tx.Commit()
	}
	if !onProForm {
		return fmt.Sprintf("paid on %s, not the CaveText Pro payment link; kept for an Admin", b.Data.FormURL), tx.Commit()
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
	case !readable || subtotal < priceSen:
		if _, err := tx.ExecContext(ctx, `UPDATE cavetext_payments SET learner_id = $2 WHERE order_number = $1`, order, learnerID); err != nil {
			return "", fmt.Errorf("mark payment's Learner: %w", err)
		}
		if !readable {
			return fmt.Sprintf("unreadable amount %q; kept for an Admin", b.Data.Main.Subtotal), tx.Commit()
		}
		return fmt.Sprintf("paid %s, under the %s price; kept for an Admin", rm(subtotal), rm(priceSen)), tx.Commit()
	}
	if err := ensureCaveTextAccount(ctx, tx, learnerID); err != nil {
		return "", err
	}
	if err := a.givePro(ctx, tx, learnerID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cavetext_payments SET learner_id = $2, granted = true WHERE order_number = $1`, order, learnerID); err != nil {
		return "", fmt.Errorf("mark payment granted: %w", err)
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

// unmatchedPayments lists the Pro payments that gave no Pro and no Admin has marked done, oldest first.
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
		WHERE order_number = $1 AND NOT granted AND done_at IS NULL`, r.PostFormValue("order")); err != nil {
		a.serverError(w, r, fmt.Errorf("mark payment done: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext", http.StatusSeeOther)
}
