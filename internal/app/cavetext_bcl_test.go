package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

const (
	bclKey      = "k3y-from-the-env"
	proFormURL  = "https://shop.bcl.my/form/cavetext-pro"
	testFormURL = "https://shop.bcl.my/form/cavetext-pro-test"
)

// bclSite is a CaveText site that takes BCL webhooks at /cavetext/bcl/<bclKey>, logged in as the Owner, with the
// CaveText Pro payment link set to proFormURL. opts may change the config further.
func bclSite(t *testing.T, opts ...func(*config.Config)) *site {
	t.Helper()
	s := newSite(t, append([]func(*config.Config){func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.CaveTextExtensionIDs = []string{extensionID}
		c.BCLWebhookKey = bclKey
	}}, opts...)...)
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return s
}

// bclPayment is the part of BCL's payment-success webhook that CAVELMS reads.
type bclPayment struct {
	event, formURL, order, payerEmail, cavelmsEmail, subtotal, kind string
	paid                                                            int
	extra                                                           map[string]any // more main_data fields, such as a coupon's
}

// paid is a paid RM25 order on the Pro form naming email in both email fields.
func paid(order, email string) bclPayment {
	return bclPayment{event: "payment-success", formURL: proFormURL, order: order, payerEmail: email, cavelmsEmail: email, subtotal: "25.00", kind: "live", paid: 1}
}

// bclPayload builds the webhook body in BCL's own shape, as captured from a Test Mode payment on 22 Oct 2026.
func bclPayload(p bclPayment) string {
	main := map[string]any{
		"payer_name": "Payer", "payer_email": p.payerEmail, "payer_telephone_number": "+60123456789",
		"order_number": p.order, "transaction_type": p.kind, "currency": "MYR",
		"subtotal_amount": p.subtotal, "fee_amount": "1.00", "amount": "26.00", "payment_channel": "FPX",
		"status": 3, "status_description": "Approved", "is_paid": p.paid, "source_type": "payment_form",
	}
	for k, v := range p.extra {
		main[k] = v
	}
	b, _ := json.Marshal(map[string]any{
		"event": p.event,
		"data": map[string]any{
			"formable_type": "payment_form", "formable_id": 19548, "form_title": "CaveText Pro (1 tahun / 1 year)",
			"form_url": p.formURL, "record_type": "Transaction", "record_id": p.order,
			"main_data":          main,
			"tracking_params":    nil,
			"custom_fields_data": map[string]any{"e_mel_akaun_cavelms_cavelms_account_email": p.cavelmsEmail},
		},
	})
	return string(b)
}

// bclPost sends body to the webhook address with key, as BCL's server would: no cookie, no browser headers.
func (s *site) bclPost(key, body string) int {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/cavetext/bcl/"+key, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.server.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func (s *site) bclPay(p bclPayment) {
	s.t.Helper()
	if code := s.bclPost(bclKey, bclPayload(p)); code != http.StatusOK {
		s.t.Fatalf("webhook for %s: status %d, want 200", p.order, code)
	}
}

func expectProForAYear(t *testing.T, l *site, token string) time.Time {
	t.Helper()
	me := l.me(token)
	if !me.Pro || me.ProUntil == nil {
		t.Fatalf("me = %+v, want Pro", me)
	}
	near(t, "Pro until", *me.ProUntil, time.Now().AddDate(1, 0, 0))
	return *me.ProUntil
}

func TestPayingOnTheProFormGivesTheLearnerAYearOfProAtOnce(t *testing.T) {
	s := bclSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ADA@example.com"))
	expectProForAYear(t, ada, token)
	_, body := s.get("/admin/log")
	if !strings.Contains(body, "LINK-1") {
		t.Errorf("the System log does not mention order LINK-1:\n%s", body)
	}
}

// Staging counts BCL Test Mode orders and ignores live ones, so the TEST forms can be tried end to end (ADR 0010).
func TestWithTestPaymentsATestModeOrderGivesProAndALiveOneDoesNot(t *testing.T) {
	s := bclSite(t, func(c *config.Config) { c.BCLTestPayments = true })
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	test := paid("LINK-1", "ada@example.com")
	test.kind = "test"
	s.bclPay(test)
	s.bclPay(paid("LINK-2", "bea@example.com"))
	expectProForAYear(t, ada, adaToken)
	if me := bea.me(beaToken); me.Pro {
		t.Fatalf("me = %+v after a live order on a site that counts only Test Mode, want free", me)
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-2 is a live payment", "nothing given")
}

func TestTheSameOrderSentTwiceGivesOneYear(t *testing.T) {
	s := bclSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	until := expectProForAYear(t, ada, token)
	s.bclPay(paid("LINK-1", "ada@example.com"))
	if got := *ada.me(token).ProUntil; !got.Equal(until) {
		t.Errorf("after a repeat of LINK-1 Pro runs until %v, want %v", got, until)
	}
	// A second, real order renews after the running year.
	s.bclPay(paid("LINK-2", "ada@example.com"))
	near(t, "Pro until after a second order", *ada.me(token).ProUntil, until.AddDate(1, 0, 0))
}

func TestALearnerWhoPaysBeforeSigningInToCaveTextHasProAtTheirFirstSignIn(t *testing.T) {
	s := bclSite(t)
	bea := s.second()
	bea.loginAsNewLearner("Bea Lim", "bea@example.com", "correct horse battery")
	s.bclPay(paid("LINK-1", "bea@example.com"))
	expectProForAYear(t, bea, bea.connect())
}

func TestTheCAVELMSEmailFieldWinsAndThePayerEmailIsTheFallback(t *testing.T) {
	s := bclSite(t)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")

	p := paid("LINK-1", "ada@example.com")
	p.payerEmail = "bea@example.com" // Bea paid for Ada
	s.bclPay(p)
	expectProForAYear(t, ada, adaToken)
	if bea.me(beaToken).Pro {
		t.Errorf("the payer got Pro, not the CAVELMS account named on the form")
	}

	p = paid("LINK-2", "typo@example.com")
	p.payerEmail = "bea@example.com"
	s.bclPay(p)
	expectProForAYear(t, bea, beaToken)
}

func TestWebhooksThatAreNotAPaidProOrderGiveNoPro(t *testing.T) {
	s := bclSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	other := paid("LINK-1", "ada@example.com")
	other.formURL = testFormURL
	other.kind = "test" // as BCL marks every order on the Test Mode copy
	unpaid := paid("LINK-2", "ada@example.com")
	unpaid.paid = 0
	submitted := paid("LINK-3", "ada@example.com")
	submitted.event = "form-submit"
	test := paid("LINK-4", "ada@example.com")
	test.kind = "test" // Test Mode switched on for the live form itself
	for _, p := range []bclPayment{other, unpaid, submitted, test} {
		s.bclPay(p)
	}
	if me := ada.me(token); me.Pro {
		t.Fatalf("me = %+v after a test-form, unpaid, submit-only and test-mode webhook, want free", me)
	}
	_, body := s.get("/admin/cavetext")
	for _, gone := range []string{"LINK-1", "LINK-2", "LINK-3", "LINK-4"} {
		if strings.Contains(body, gone) {
			t.Errorf("%s, a test or unpaid order, is listed as unmatched:\n%s", gone, body)
		}
	}
	if code := s.bclPost(bclKey, "not json"); code != http.StatusBadRequest {
		t.Errorf("a body that is not JSON: status %d, want 400", code)
	}
}

func TestAPaymentNamingNoLearnerWaitsOnTheAdminPageUntilMarkedDone(t *testing.T) {
	s := bclSite(t)
	s.bclPay(paid("LINK-7", "nobody@example.com"))
	elsewhere := paid("LINK-9", "ada@example.com")
	elsewhere.formURL = "https://shop.bcl.my/form/old-pro-form" // still taking money after the Payment link moved
	short := paid("LINK-8", "ada@example.com")
	short.subtotal = "10.00" // less than the price
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(short)
	s.bclPay(elsewhere)
	if ada.me(token).Pro {
		t.Fatalf("a RM10 payment, or one on another form, gave Pro")
	}

	res, body := s.get("/admin/cavetext")
	expectStatus(t, res, body, http.StatusOK, "Payments not matched")
	for _, want := range []string{"LINK-7", "nobody@example.com", "LINK-8", "RM10", "LINK-9"} {
		if !strings.Contains(body, want) {
			t.Errorf("the unmatched payments lack %q:\n%s", want, body)
		}
	}
	for order, product := range map[string]string{"LINK-7": "CaveText Pro", "LINK-8": "CaveText Pro", "LINK-9": "-"} {
		if got := unmatchedProduct(t, body, order); got != product {
			t.Errorf("%s is shown as for %q, want %q", order, got, product)
		}
	}
	if !regexp.MustCompile(`action="/admin/cavetext/payments/done"[^>]*>\s*<input type="hidden" name="order" value="LINK-7">`).MatchString(body) {
		t.Fatalf("no Done button for LINK-7:\n%s", body)
	}
	res, body = s.post("/admin/cavetext/payments/done", url.Values{"order": {"LINK-7"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/cavetext")
	if strings.Contains(body, "LINK-7") || !strings.Contains(body, "LINK-8") {
		t.Errorf("after Done, LINK-7 should be gone and LINK-8 still listed:\n%s", body)
	}

	// A Learner can't mark one done.
	res, body = ada.post("/admin/cavetext/payments/done", url.Values{"order": {"LINK-8"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	if _, body = s.get("/admin/cavetext"); !strings.Contains(body, "LINK-8") {
		t.Errorf("a Learner marked LINK-8 done")
	}
}

// unmatchedProduct reads which product's form the unmatched payment order came from, as the Admin's CaveText
// Learners page shows it.
func unmatchedProduct(t *testing.T, body, order string) string {
	t.Helper()
	m := regexp.MustCompile(`<td>` + order + `</td>[\s\S]*?<td class="product">([^<]*)</td>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no product shown for %s:\n%s", order, body)
	}
	return m[1]
}

func TestABCLWebhookWithAWrongKeyIsRefused(t *testing.T) {
	s := bclSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	for _, key := range []string{"wrong", ""} {
		if code := s.bclPost(key, bclPayload(paid("FAKE-9", "ada@example.com"))); code != http.StatusNotFound {
			t.Errorf("webhook with key %q: status %d, want 404", key, code)
		}
	}
	if ada.me(token).Pro {
		t.Errorf("a webhook with a wrong key gave Pro")
	}
}

func TestBCLWebhooksAreRefusedWhenNoKeyIsSet(t *testing.T) {
	s := ownerSite(t)
	if code := s.bclPost("", bclPayload(paid("FAKE-1", "a@example.com"))); code != http.StatusNotFound {
		t.Errorf("webhook with no key set: status %d, want 404", code)
	}
}

func TestAFailedBCLWebhookIsLoggedWithoutItsKeyAndAnswered500SoBCLRetries(t *testing.T) {
	s := bclSite(t)
	if _, err := s.db.Exec(`ALTER TABLE cavetext_payments RENAME TO cavetext_payments_gone`); err != nil {
		t.Fatal(err)
	}
	code := s.bclPost(bclKey, bclPayload(paid("LINK-5", "ada@example.com")))
	if _, err := s.db.Exec(`ALTER TABLE cavetext_payments_gone RENAME TO cavetext_payments`); err != nil {
		t.Fatal(err)
	}
	if code != http.StatusInternalServerError {
		t.Errorf("webhook with the payments table gone: status %d, want 500", code)
	}
	_, body := s.get("/admin/log")
	if !strings.Contains(body, "LINK-5") {
		t.Errorf("the System log does not mention the failed order:\n%s", body)
	}
	if strings.Contains(body, bclKey) {
		t.Errorf("the System log holds the webhook key:\n%s", body)
	}
}

func TestBCLAmountsSentAsNumbersStillCount(t *testing.T) {
	s := bclSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	body := strings.Replace(bclPayload(paid("LINK-1", "ada@example.com")), `"subtotal_amount":"25.00"`, `"subtotal_amount":25`, 1)
	if code := s.bclPost(bclKey, body); code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	expectProForAYear(t, ada, token)
}

func TestATestModePaymentIsLoggedWithItsAmountsAndAnyCoupon(t *testing.T) {
	s := bclSite(t)
	test := paid("LINK-5", "ada@example.com")
	test.formURL, test.kind, test.subtotal = testFormURL, "test", "249.00"
	test.extra = map[string]any{"amount": "100.00", "discount_amount": "150.00", "coupon_code": "CAVETEST"}
	s.bclPay(test)
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-5 is a Test Mode payment", "subtotal 249.00", "amount 100.00",
		"coupon_code=CAVETEST", "discount_amount=150.00", "nothing given")
	expectNone(t, body, "no Pro given")
}

const bclToken = "bcl-api-read-token"

// The titles of the live BCL forms, as BCL's form API reports them; an order's remarks name the one it was paid on.
const (
	proFormTitle     = "CaveText Pro (1 tahun / 1 year)"
	labPlanFormTitle = "Lab Plan (1 tahun / 1 year)"
	proPlanFormTitle = "Pro Plan (1 tahun / 1 year)"
)

// bclOrder is an order as BCL's transaction API reports it; form is the title of the form it was paid on.
type bclOrder struct{ amount, email, status, form string }

// confirmed is how BCL reports paid(order, email): RM26 charged, successful, on the CaveText Pro form.
func confirmed(email string) bclOrder {
	return bclOrder{amount: "26.00", email: email, status: "3", form: proFormTitle}
}

// fakeBCL is BCL's API. /transactions?search= lists the live orders whose number contains the search, as loosely as
// BCL may; /transaction/<order> reports one order (Test Mode too) or 404; /forms lists forms by slug and title. It
// answers 503 to everything while down.
type fakeBCL struct {
	url    string
	orders map[string]bclOrder
	forms  map[string]string // slug to title
	mu     sync.Mutex
	down   bool
	asked  []*http.Request
}

func newFakeBCL(t *testing.T, orders map[string]bclOrder) *fakeBCL {
	t.Helper()
	f := &fakeBCL{orders: orders, forms: map[string]string{
		"cavetext-pro": proFormTitle, "lab-plan": labPlanFormTitle, "pro-plan": proPlanFormTitle}}
	// BCL sends a list's amount as a number; one that is not a number stays a string, as BCL would never send it.
	amount := func(s string) any {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return json.Number(s)
		}
		return s
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked = append(f.asked, r)
		if f.down {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch {
		case r.URL.Path == "/forms":
			var forms []any
			for slug, title := range f.forms {
				forms = append(forms, map[string]any{"form_id": 19000, "slug": slug, "title": title, "status": "active"})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": forms})
		case r.URL.Path == "/transactions":
			rows := []any{}
			for number, o := range f.orders {
				if strings.Contains(number, r.URL.Query().Get("search")) {
					code, _ := strconv.Atoi(o.status)
					rows = append(rows, map[string]any{"id": 1, "order_number": number, "status": "success", "status_code": code,
						"amount": amount(o.amount), "payer_email": o.email, "payer_name": "Payer", "form_type": "Payment Form",
						"remarks": o.form, "items": []any{}})
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": rows, "meta": map[string]any{"total": len(rows)}})
		case strings.HasPrefix(r.URL.Path, "/transaction/"):
			number := strings.TrimPrefix(r.URL.Path, "/transaction/")
			o, known := f.orders[number]
			if !known {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"message":"Transaction not found"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
				"transaction": map[string]any{"order_number": number, "amount": o.amount, "payer_name": "Payer",
					"payer_email": o.email, "payer_telephone_number": "+60123456789", "status": o.status,
					"created_at": "23 Oct 2026, 12:31 PM", "updated_at": "23 Oct 2026, 12:32 PM"},
				"payment_status": "Successful"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// setDown makes BCL's API answer 503 while down.
func (f *fakeBCL) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

// setOrder makes BCL's API report o for number from now on.
func (f *fakeBCL) setOrder(number string, o bclOrder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orders == nil {
		f.orders = map[string]bclOrder{}
	}
	f.orders[number] = o
}

// renameForm gives the form slug a new title at BCL from now on.
func (f *fakeBCL) renameForm(slug, title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forms[slug] = title
}

// lastAsked is the last request BCL's API received at path.
func (f *fakeBCL) lastAsked(t *testing.T, path string) *http.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.asked) - 1; i >= 0; i-- {
		if f.asked[i].URL.Path == path {
			return f.asked[i]
		}
	}
	t.Fatalf("BCL's API was never asked for %s", path)
	return nil
}

// confirmingSite is bclSite with a BCL API token, so each order is looked up in bcl before it gives anything.
func confirmingSite(t *testing.T, bcl *fakeBCL, opts ...func(*config.Config)) *site {
	t.Helper()
	return bclSite(t, append([]func(*config.Config){func(c *config.Config) {
		c.BCLAPIToken = bclToken
		c.BCLAPIURL = bcl.url
	}}, opts...)...)
}

// An order BCL does not know yet, or has not settled, may be a real payment BCL is still writing down: it is answered
// 500, so BCL retries, and gives nothing until BCL confirms it.
func TestBCLDoesNotKnowTheOrderYetSo500ThenOnRetryItIsConfirmedAndGranted(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-10": confirmed("ada@example.com")}) // listed by a search for LINK-1
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook for an order BCL does not know: status %d, want 500", code)
	}
	bcl.setOrder("LINK-1", bclOrder{amount: "26.00", email: "ada@example.com", status: "1"}) // pending
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook for an order pending at BCL: status %d, want 500", code)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order BCL has not confirmed gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectNone(t, body, "LINK-1")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL: BCL does not know it", "order LINK-1: confirm with BCL: it has status 1 at BCL")
	expectNone(t, body, bclToken, bclKey)

	bcl.setOrder("LINK-1", confirmed("ada@example.com"))
	s.bclPay(paid("LINK-1", "ada@example.com")) // BCL's retry
	expectProForAYear(t, ada, token)
}

func TestAnOrderBCLConfirmsGivesProAndIsLookedUpWithTheToken(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed(" Ada@Example.com ")})
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	expectProForAYear(t, ada, token)
	asked := bcl.lastAsked(t, "/transactions") // which lists live orders
	if got := asked.URL.Query().Get("search"); got != "LINK-1" {
		t.Errorf("searched BCL's orders for %q, want LINK-1", got)
	}
	if got := asked.Header.Get("Authorization"); got != "Bearer "+bclToken {
		t.Errorf("Authorization = %q, want the token as a Bearer", got)
	}
	if got := asked.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want application/json", got)
	}
	if ua := asked.Header.Get("User-Agent"); ua == "" || strings.HasPrefix(ua, "Go-http-client") {
		t.Errorf("User-Agent = %q, which BCL blocks", ua)
	}
}

// Staging counts Test Mode orders, which BCL's /transactions/ does not find, so it asks /transaction/ (ADR 0010).
func TestWithTestPaymentsATestModeOrderIsLookedUpWhereBCLFindsTestOrders(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	test := paid("LINK-1", "ada@example.com")
	test.kind = "test"
	s.bclPay(test)
	expectProForAYear(t, ada, token)
	bcl.lastAsked(t, "/transaction/LINK-1")
}

func TestAnOrderBCLReportsDifferentlyIsLoggedAndGivesNothing(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{
		"LINK-1": {amount: "1.00", email: "ada@example.com", status: "3"},   // cheaper than the webhook says
		"LINK-2": {amount: "26.00", email: "eve@example.com", status: "3"},  // someone else's order
		"LINK-4": {amount: "26.00", email: "ada@example.com", status: "3"},  // real, but the body claims a dearer order
		"LINK-5": {amount: "twenty", email: "ada@example.com", status: "3"}, // unreadable
		"LINK-6": {amount: "26.00", email: "ada@example.com", status: "3"},  // real, but the body's subtotal is unreadable
	})
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	dearer := paid("LINK-4", "ada@example.com")
	dearer.subtotal = "249.00" // what a Pro Plan costs, though BCL charged RM26
	unreadable := paid("LINK-6", "ada@example.com")
	unreadable.subtotal = "lots"
	for _, p := range []bclPayment{paid("LINK-1", "ada@example.com"), paid("LINK-2", "ada@example.com"),
		dearer, paid("LINK-5", "ada@example.com"), unreadable} {
		s.bclPay(p)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order BCL reports differently gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectNone(t, body, "LINK-1", "LINK-2", "LINK-4", "LINK-5", "LINK-6")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1 is for 1.00 at BCL, not 26.00", "order LINK-2 was paid by eve@example.com at BCL, not ada@example.com",
		"order LINK-4 charged 26.00 at BCL, under the RM249", "order LINK-5 is for twenty at BCL, not 26.00",
		"order LINK-6 claims an unreadable subtotal or discount")
	expectNone(t, body, bclToken)
}

func TestWhileBCLIsDownAnOrderIsAnswered500SoBCLRetriesAndGivesNothingUntilThen(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	bcl.setDown(true)
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook while BCL is down: status %d, want 500", code)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order BCL could not confirm gave Pro")
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL")
	expectNone(t, body, bclToken)

	bcl.setDown(false)
	s.bclPay(paid("LINK-1", "ada@example.com")) // BCL's retry
	expectProForAYear(t, ada, token)
}

// A real order on the Lab Plan form, replayed with the Pro Plan's form address, must not give the Pro Plan, even when
// what it paid covers the Pro Plan's price: BCL says which form the order was paid on.
func TestARealOrderReplayedWithAnotherFormsAddressGivesNothing(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{
		"LINK-12": confirmed("ada@example.com"), // a near match BCL's search also lists
		"LINK-1":  {amount: "90.00", email: "ada@example.com", status: "3", form: labPlanFormTitle},
	})
	s := confirmingSite(t, bcl)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "89", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	replay := paid("LINK-1", "ada@example.com")
	replay.formURL, replay.subtotal = proPlanFormURL, "89.00"
	replay.extra = map[string]any{"amount": "90.00"}
	s.bclPay(replay)
	if ada.me(token).Pro {
		t.Fatalf("a Lab Plan order replayed on the Pro Plan form gave CaveText Pro")
	}
	_, body = s.get("/admin/cavetext")
	expectNone(t, body, "LINK-1")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1 was paid on BCL form", labPlanFormTitle, proPlanFormTitle, "nothing given")
}

// Form titles are read from BCL each time, so a form renamed at BCL keeps taking payments under its new title.
func TestAnOrderOnAFormRenamedAtBCLStillGivesPro(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com")) // read under the old title
	bcl.renameForm("cavetext-pro", "CaveText Pro (2027)")
	o := confirmed("ada@example.com")
	o.form = "CaveText Pro (2027)"
	bcl.setOrder("LINK-2", o)
	s.bclPay(paid("LINK-2", "ada@example.com"))
	near(t, "Pro until after an order on the renamed form", *ada.me(token).ProUntil, time.Now().AddDate(2, 0, 0))
}
