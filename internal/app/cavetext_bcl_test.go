package app_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	noCustomFields                                                  bool           // custom_fields_data is [], as from a form without them
}

// paid is a paid RM25 order on the Pro form naming email in both email fields.
func paid(order, email string) bclPayment {
	return bclPayment{event: "payment-success", formURL: proFormURL, order: order, payerEmail: email, cavelmsEmail: email, subtotal: "25.00", kind: "live", paid: 1}
}

// testOrder is a paid Test Mode order on the CaveText Pro form naming email in both email fields, as staging takes it.
func testOrder(order, email string) bclPayment {
	p := paid(order, email)
	p.kind = "test"
	return p
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
	var custom any = map[string]any{"e_mel_akaun_cavelms_cavelms_account_email": p.cavelmsEmail}
	if p.noCustomFields {
		custom = []any{}
	}
	b, _ := json.Marshal(map[string]any{
		"event": p.event,
		"data": map[string]any{
			"formable_type": "payment_form", "formable_id": 19548, "form_title": "CaveText Pro (1 tahun / 1 year)",
			"form_url": p.formURL, "record_type": "Transaction", "record_id": p.order,
			"main_data":          main,
			"tracking_params":    nil,
			"custom_fields_data": custom,
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
	s.bclPay(testOrder("LINK-1", "ada@example.com"))
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
	other := testOrder("LINK-1", "ada@example.com") // as BCL marks every order on the Test Mode copy
	other.formURL = testFormURL
	unpaid := paid("LINK-2", "ada@example.com")
	unpaid.paid = 0
	submitted := paid("LINK-3", "ada@example.com")
	submitted.event = "form-submit"
	test := paid("LINK-4", "ada@example.com")
	test.kind = "Test" // Test Mode switched on for the live form itself; BCL's case is not relied on
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
	test := testOrder("LINK-5", "ada@example.com")
	test.formURL, test.subtotal = testFormURL, "249.00"
	test.extra = map[string]any{"amount": "100.00", "discount_amount": "150.00", "coupon_code": "CAVETEST"}
	s.bclPay(test)
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-5 is a Test Mode payment", "subtotal 249.00", "amount 100.00",
		"coupon_code=CAVETEST", "discount_amount=150.00", "nothing given")
	expectNone(t, body, "no Pro given")
}

const bclToken = "bcl-api-read-token"

// The ids of the live BCL forms, as BCL's form API and an order's formable_id report them.
const (
	proFormID     = 19548
	labPlanFormID = 19613
	proPlanFormID = 19615
)

// bclOrder is an order as BCL's transaction API reports it. formID is the form it was paid on; cavelms is its CAVELMS
// account email field, empty for none.
type bclOrder struct {
	amount, subtotal, discount, payer, cavelms, status, kind string
	paid                                                     bool
	formID                                                   int
}

// confirmed is how BCL reports paid(order, email): RM25 plus a RM1 fee, successful and live, on the CaveText Pro form.
func confirmed(email string) bclOrder {
	return bclOrder{amount: "26.00", subtotal: "25.00", payer: email, cavelms: email, status: "3", kind: "live", paid: true, formID: proFormID}
}

// bclForm is a form as BCL's form API lists it.
type bclForm struct {
	id    int
	title string
}

// fakeBCL is BCL's API. /transactions/<order> gives an order's detail or 404; /transaction/<order> reports one order
// (Test Mode too) without its form or custom fields, or 404; /forms lists forms by id, slug and title. It answers 503 to
// everything while down.
type fakeBCL struct {
	url       string
	orders    map[string]bclOrder
	forms     map[string]bclForm // by slug
	mu        sync.Mutex
	down      bool
	formsDown bool // only /forms answers 503
	refuse    int  // a status, 401 or 403, everything answers when BCL refuses the token; 0 for none
	asked     []*http.Request
}

func newFakeBCL(t *testing.T, orders map[string]bclOrder) *fakeBCL {
	t.Helper()
	f := &fakeBCL{orders: orders, forms: map[string]bclForm{
		"cavetext-pro": {proFormID, "CaveText Pro (1 tahun / 1 year)"},
		"lab-plan":     {labPlanFormID, "Lab Plan (1 tahun / 1 year)"},
		"pro-plan":     {proPlanFormID, "Pro Plan (1 tahun / 1 year)"},
	}}
	// BCL sends an order's amounts as numbers; one that is not a number stays a string, as BCL would never send it.
	number := func(s string) any {
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
		if f.refuse != 0 {
			http.Error(w, `{"message":"Unauthenticated."}`, f.refuse)
			return
		}
		notFound := func() {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Transaction not found"}`))
		}
		switch {
		case r.URL.Path == "/forms" && f.formsDown:
			http.Error(w, "down", http.StatusServiceUnavailable)
		case r.URL.Path == "/forms":
			var forms []any
			for slug, form := range f.forms {
				var id any // null for a form with no id
				if form.id != 0 {
					id = form.id
				}
				forms = append(forms, map[string]any{"form_id": id, "slug": slug, "title": form.title,
					"form_url": "https://shop.bcl.my/form/" + slug, "status": "active"})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": forms})
		case strings.HasPrefix(r.URL.Path, "/transactions/"):
			order := strings.TrimPrefix(r.URL.Path, "/transactions/")
			o, known := f.orders[order]
			if !known {
				notFound()
				return
			}
			var custom any = []any{} // PHP's empty object
			if o.cavelms != "" {
				custom = map[string]any{"e_mel_akaun_cavelms_cavelms_account_email": o.cavelms}
			}
			code, _ := strconv.Atoi(o.status)
			var formID any // null when the order names no form
			if o.formID != 0 {
				formID = o.formID
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
				"record_id": order, "record_type": "Transaction", "formable_id": formID, "formable_type": "payment_form",
				"form_title": "some form", "custom_fields_data": custom,
				"customer":    map[string]any{"id": 1, "email": o.payer, "name": "Payer", "phone": "+60123456789"},
				"receipt_url": "https://bcl.my/receipt/1",
				"main_data": map[string]any{"id": 1, "order_number": order, "payer_name": "BCL Payer", "payer_email": o.payer,
					"payer_telephone_number": "+60123456789", "transaction_type": o.kind, "currency": "MYR",
					"subtotal_amount": number(o.subtotal), "discount_amount": number(o.discount), "fee_amount": 1,
					"additional_fee_amount": 0, "amount": number(o.amount), "payment_channel": "FPX", "status": code,
					"status_description": "Successful", "is_paid": o.paid, "items": []any{}},
				"team": map[string]any{"id": 1}}})
		case strings.HasPrefix(r.URL.Path, "/transaction/"):
			order := strings.TrimPrefix(r.URL.Path, "/transaction/")
			o, known := f.orders[order]
			if !known {
				notFound()
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
				"transaction": map[string]any{"order_number": order, "amount": o.amount, "payer_name": "Payer",
					"payer_email": o.payer, "payer_telephone_number": "+60123456789", "status": o.status,
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

// setRefuse makes BCL answer status (401 or 403) to everything, as for a revoked or wrong token; 0 accepts it again.
func (f *fakeBCL) setRefuse(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refuse = status
}

// setFormsDown makes BCL's /forms alone answer 503 while down.
func (f *fakeBCL) setFormsDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formsDown = down
}

// askedFor counts the requests BCL's API received at a path starting with prefix.
func (f *fakeBCL) askedFor(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.asked {
		if strings.HasPrefix(r.URL.Path, prefix) {
			n++
		}
	}
	return n
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

// addForm adds a form to BCL from now on.
func (f *fakeBCL) addForm(slug string, form bclForm) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forms[slug] = form
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

// The order's detail at BCL names the CAVELMS account, so a webhook without the field still gives it to that account.
func TestAnOrderBCLConfirmsGoesToBCLsCAVELMSEmailAndIsLookedUpWithTheToken(t *testing.T) {
	o := confirmed("bea@example.com")
	o.cavelms = " Ada@Example.com " // Bea paid for Ada
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": o})
	s := confirmingSite(t, bcl)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	p := paid("LINK-1", "bea@example.com")
	p.cavelmsEmail = "" // the webhook leaves the field out
	s.bclPay(p)
	expectProForAYear(t, ada, adaToken)
	if bea.me(beaToken).Pro {
		t.Errorf("the payer got Pro, not the CAVELMS account BCL's order names")
	}
	asked := bcl.lastAsked(t, "/transactions/LINK-1")
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

// Someone with the webhook key who knows a real order can race BCL's own webhook with another CAVELMS email. BCL's
// order names the account, so that one gets it and the forged one nothing; the System log names neither email.
func TestAWebhookNamingAnotherCAVELMSAccountThanBCLsOrderGivesItToBCLs(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	eve, eveToken := s.newCaveTextLearner("Eve Mallory", "eve@example.com")
	forged := paid("LINK-1", "ada@example.com")
	forged.cavelmsEmail = "eve@example.com"
	s.bclPay(forged)
	s.bclPay(paid("LINK-1", "ada@example.com")) // BCL's own webhook, second
	if eve.me(eveToken).Pro {
		t.Fatalf("a forged CAVELMS email got Pro from someone else's order")
	}
	expectProForAYear(t, ada, adaToken)
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-1: gave Learner", "another CAVELMS account than BCL", "order LINK-1: already received")
	expectNone(t, body, "eve@example.com", "ada@example.com")
}

// BCL's amounts and payer decide, not the webhook's: a body claiming less still gives what BCL says was paid, and one
// claiming more is held at what BCL says. Each difference is logged.
func TestAnOrderIsTakenAtBCLsAmountsNotTheWebhooks(t *testing.T) {
	short := confirmed("bea@example.com")
	short.subtotal, short.amount = "10", "11"
	under := confirmed("bea@example.com")
	under.amount = "20" // BCL's charge must cover its subtotal less discount
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com"), "LINK-2": short, "LINK-3": under})
	s := confirmingSite(t, bcl)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	low := paid("LINK-1", "ada@example.com")
	low.subtotal, low.payerEmail = "10.00", "someone@example.com"
	s.bclPay(low)
	s.bclPay(paid("LINK-2", "bea@example.com")) // claims RM25
	s.bclPay(paid("LINK-3", "bea@example.com"))
	expectProForAYear(t, ada, adaToken)
	if bea.me(beaToken).Pro {
		t.Fatalf("an order BCL says paid RM10, or charged under its subtotal, gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectAll(t, body, "LINK-2", "RM10")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: gave Learner", "another payer email than BCL", "subtotal 10.00, BCL 25.00",
		"order LINK-2: paid RM10, under the", "subtotal 25.00, BCL 10", "amount 26.00, BCL 11",
		"order LINK-3 charged 20 at BCL, under the RM25 it paid before the fee; kept for an Admin")
	expectNone(t, body, "someone@example.com", "ada@example.com")
}

// A forged body naming a real Lab Plan order with the Pro Plan's form address, racing BCL's own webhook, gives what BCL
// says the order bought, to the account BCL's order names: BCL's form id picks the product, not the webhook's address.
// BCL's real webhook after it is a repeat.
func TestAForgedWebhookNamingARealOrderWithAnotherFormsAddressGivesWhatBCLsFormSells(t *testing.T) {
	lab := confirmed("ada@example.com")
	lab.subtotal, lab.amount, lab.formID = "89.00", "90.00", labPlanFormID
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": lab})
	s := confirmingSite(t, bcl)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "89", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.createPlan("Lab Month", "30", "5")
	s.markLabPlan("Lab Month")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.newCaveTextLearner("Eve Mallory", "eve@example.com")
	forged := paid("LINK-1", "eve@example.com")
	forged.formURL, forged.subtotal = proPlanFormURL, "89.00"
	forged.extra = map[string]any{"amount": "90.00"}
	s.bclPay(forged)
	lab2 := paidLabPlan("LINK-1", "ada@example.com")
	lab2.extra = map[string]any{"amount": "90.00"}
	s.bclPay(lab2) // BCL's own webhook, second
	if ada.me(token).Pro {
		t.Fatalf("a Lab Plan order sent with the Pro Plan's address gave CaveText Pro")
	}
	if periods := s.periodsOf("ada@example.com"); len(periods) != 1 {
		t.Fatalf("Ada's periods = %+v, want the one Lab Plan period", periods)
	}
	if periods := s.periodsOf("eve@example.com"); len(periods) != 0 {
		t.Fatalf("Eve's periods = %+v, want none", periods)
	}
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: gave Learner", "lab-plan", "form address "+proPlanFormURL+", paid on "+labFormURL,
		"order LINK-1: already received")
}

// An order names its form by id, so two BCL forms with one title are still told apart: one paid on a form that is no
// product's is kept for an Admin, whatever form address the webhook gives.
func TestAnOrderOnABCLFormThatIsNoProductsIsKeptForAnAdmin(t *testing.T) {
	bcl := newFakeBCL(t, nil)
	bcl.addForm("cavetext-pro-copy", bclForm{19999, "CaveText Pro (1 tahun / 1 year)"}) // same title as cavetext-pro
	copied := confirmed("ada@example.com")
	copied.formID = 19999
	bcl.setOrder("LINK-1", copied)
	gone := confirmed("ada@example.com")
	gone.formID = 20000 // a form BCL no longer lists
	bcl.setOrder("LINK-2", gone)
	bcl.setOrder("LINK-3", confirmed("bea@example.com"))
	s := confirmingSite(t, bcl)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com")) // sent with the Pro form's address
	s.bclPay(paid("LINK-2", "ada@example.com"))
	s.bclPay(paid("LINK-3", "bea@example.com"))
	if ada.me(adaToken).Pro {
		t.Fatalf("an order on a form that is no product's gave Pro")
	}
	expectProForAYear(t, bea, beaToken)
	_, body := s.get("/admin/cavetext")
	for _, order := range []string{"LINK-1", "LINK-2"} {
		if got := unmatchedProduct(t, body, order); got != "-" {
			t.Errorf("%s is shown as for %q, want -", order, got)
		}
	}
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: paid on BCL form 19999, no current payment link; kept for an Admin",
		"form address "+proFormURL+", paid on BCL form 19999", "order LINK-2: paid on BCL form 20000")
}

// BCL's case for an order's transaction type is not relied on.
func TestAnOrderBCLHasAsLiveInAnyCaseGivesPro(t *testing.T) {
	o := confirmed("ada@example.com")
	o.kind = "LIVE"
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": o})
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	expectProForAYear(t, ada, token)
}

// BCL may send the form's address with a trailing slash, a query string or another case: it is still that form.
func TestAnOrderWhoseFormAddressDiffersOnlyInSlashQueryOrCaseStillGivesPro(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	p := paid("LINK-1", "ada@example.com")
	p.formURL = "https://shop.bcl.my/form/CaveText-Pro/?utm_source=ig"
	s.bclPay(p)
	expectProForAYear(t, ada, token)
}

// An order BCL does not know yet, or has not settled as paid and live, may be a real payment BCL is still writing
// down: it is answered 500, so BCL retries, and gives nothing until BCL confirms it.
func TestBCLDoesNotKnowTheOrderYetSo500ThenOnRetryItIsConfirmedAndGranted(t *testing.T) {
	bcl := newFakeBCL(t, nil)
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	pending := confirmed("ada@example.com")
	pending.status = "1"
	unpaid := confirmed("ada@example.com")
	unpaid.paid = false
	test := confirmed("ada@example.com")
	test.kind = "test"
	for i, o := range []*bclOrder{nil, &pending, &unpaid, &test} {
		if o != nil {
			bcl.setOrder("LINK-1", *o)
		}
		if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
			t.Fatalf("webhook %d for an order BCL has not confirmed: status %d, want 500", i, code)
		}
	}
	if ada.me(token).Pro {
		t.Fatalf("an order BCL has not confirmed gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectNone(t, body, "LINK-1")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL: BCL does not know it",
		"order LINK-1: confirm with BCL: it has status 1, is_paid true, type live at BCL",
		"it has status 3, is_paid false, type live", "it has status 3, is_paid true, type test")
	expectNone(t, body, bclToken, bclKey)

	bcl.setOrder("LINK-1", confirmed("ada@example.com"))
	s.bclPay(paid("LINK-1", "ada@example.com")) // BCL's retry
	expectProForAYear(t, ada, token)
}

// An order whose detail names no form is never matched to a form BCL lists without an id: BCL retries, and a form
// listed with no id gives nothing.
func TestAnOrderWhoseDetailNamesNoFormIsAnswered500(t *testing.T) {
	noForm := confirmed("ada@example.com")
	noForm.formID = 0
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": noForm})
	bcl.addForm("cavetext-pro", bclForm{}) // listed without an id
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook for an order naming no form: status %d, want 500", code)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order naming no form gave Pro")
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL: BCL", "names no form")
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

// The form check reading BCL's forms is part of confirming the order: BCL failing there is answered 500 too, so BCL
// retries and the real payment is given then.
func TestWhileBCLsFormsAreDownAnOrderIsAnswered500AndGivenOnRetry(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	bcl.setFormsDown(true)
	s := confirmingSite(t, bcl)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook while BCL's forms are down: status %d, want 500", code)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order whose form could not be checked gave Pro")
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL: read BCL", "forms: BCL answered 503")

	bcl.setFormsDown(false)
	s.bclPay(paid("LINK-1", "ada@example.com")) // BCL's retry
	expectProForAYear(t, ada, token)
}

// Staging counts Test Mode orders, which BCL's /transactions/<order> does not find, so it asks /transaction/<order>,
// which names no form and no custom fields: staging asks BCL for neither (ADR 0010).
func TestWithTestPaymentsATestModeOrderIsLookedUpWhereBCLFindsTestOrders(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	atStart := bcl.askedFor("/forms") // the startup token check reads them
	s.bclPay(testOrder("LINK-1", "ada@example.com"))
	expectProForAYear(t, ada, token)
	bcl.lastAsked(t, "/transaction/LINK-1")
	if n := bcl.askedFor("/forms") - atStart + bcl.askedFor("/transactions"); n != 0 {
		t.Errorf("staging asked BCL for its forms or an order's detail %d times, want none", n)
	}
}

// On staging BCL's /transaction/<order> gives no amounts but the charge, so the webhook's must agree with it.
func TestWithTestPaymentsAnOrderBCLReportsDifferentlyIsLoggedAndGivesNothing(t *testing.T) {
	order := func(amount, payer string) bclOrder { return bclOrder{amount: amount, payer: payer, status: "3"} }
	bcl := newFakeBCL(t, map[string]bclOrder{
		"LINK-1": order("1.00", "ada@example.com"),   // cheaper than the webhook says
		"LINK-2": order("26.00", "eve@example.com"),  // someone else's order
		"LINK-4": order("26.00", "ada@example.com"),  // real, but the body claims a dearer order
		"LINK-5": order("twenty", "ada@example.com"), // unreadable
		"LINK-6": order("26.00", "ada@example.com"),  // real, but the body's subtotal is unreadable
	})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	test := func(number string) bclPayment { return testOrder(number, "ada@example.com") }
	dearer := test("LINK-4")
	dearer.subtotal = "249.00" // what a Pro Plan costs, though BCL charged RM26
	unreadable := test("LINK-6")
	unreadable.subtotal = "lots"
	for _, p := range []bclPayment{test("LINK-1"), test("LINK-2"), dearer, test("LINK-5"), unreadable} {
		s.bclPay(p)
	}
	if ada.me(token).Pro {
		t.Fatalf("an order BCL reports differently gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectNone(t, body, "LINK-1", "LINK-2", "LINK-4", "LINK-5", "LINK-6")
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1 is for 1.00 at BCL, not 26.00", "order LINK-2 was paid by another payer email at BCL than the webhook",
		"order LINK-4 charged 26.00 at BCL, under the RM249", "order LINK-5 is for twenty at BCL, not 26.00",
		"order LINK-6 claims an unreadable subtotal or discount")
	expectNone(t, body, bclToken, "eve@example.com", "ada@example.com")
}

// BCL gives nothing but the payer for a Test Mode order, so on staging the CAVELMS account the webhook names must be
// BCL's payer: one naming another account is kept for an Admin, linked to BCL's payer's account, gives nothing to
// either, and the System log names neither email.
func TestWithTestPaymentsAWebhookNamingAnotherCAVELMSAccountThanBCLsPayerIsKeptForAnAdmin(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	eve, eveToken := s.newCaveTextLearner("Eve Mallory", "eve@example.com")
	forged := testOrder("LINK-1", "ada@example.com")
	forged.cavelmsEmail = "eve@example.com"
	s.bclPay(forged)
	s.bclPay(testOrder("LINK-1", "ada@example.com")) // BCL's own webhook, second
	if eve.me(eveToken).Pro || ada.me(adaToken).Pro {
		t.Fatalf("a Test Mode order naming another CAVELMS account than BCL's payer gave Pro")
	}
	_, body := s.get("/admin/cavetext")
	expectAll(t, body, "Payments not matched", "LINK-1")
	var linked string // the page shows no Learner, so the held row's is read where it is kept
	if err := s.db.QueryRow(`SELECT l.email FROM cavetext_payments p JOIN learners l ON l.id = p.learner_id
		WHERE p.order_number = 'LINK-1'`).Scan(&linked); err != nil || linked != "ada@example.com" {
		t.Fatalf("the held order is linked to %q (%v), want BCL's payer's account ada@example.com", linked, err)
	}
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1 the webhook names another CAVELMS account than BCL", "s payer; kept for an Admin",
		"order LINK-1: already received")
	expectNone(t, body, "eve@example.com", "ada@example.com")
}

// On staging the CAVELMS account email matches BCL's payer ignoring case and spaces, and a webhook with no CAVELMS
// email field gives the order to BCL's payer.
func TestWithTestPaymentsTheCAVELMSEmailMatchesBCLsPayerInAnyCaseAndItsAbsenceFallsBackToThePayer(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com"), "LINK-2": confirmed("bea@example.com")})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	p := testOrder("LINK-1", "ada@example.com")
	p.cavelmsEmail = " ADA@Example.com "
	s.bclPay(p)
	noField := testOrder("LINK-2", "bea@example.com")
	noField.noCustomFields = true
	s.bclPay(noField)
	expectProForAYear(t, ada, adaToken)
	expectProForAYear(t, bea, beaToken)
}

// On staging BCL gives no form for a Test Mode order, so the form is bound only by the amount and the price: a cheaper
// order sent with a dearer TEST form's address gives nothing, and a dearer one sent with a cheaper form's address
// gives only the cheaper product.
func TestWithTestPaymentsAnOrderSentWithAnotherTESTFormsAddressGivesNoMoreThanItPaidFor(t *testing.T) {
	const proPlanTestFormURL = "https://shop.bcl.my/form/pro-plan-test"
	dear := bclOrder{amount: "250.00", payer: "bea@example.com", status: "3"}
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com"), "LINK-2": dear})
	s := confirmingSite(t, bcl, func(c *config.Config) { c.BCLTestPayments = true })
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": testFormURL, "pro-plan_link": proPlanTestFormURL,
		"pro-plan_price": "249", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.createPlan("Lab Month", "30", "5")
	s.markLabPlan("Lab Month")
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	cheap := testOrder("LINK-1", "ada@example.com") // a RM25 CaveText Pro order
	cheap.formURL = proPlanTestFormURL
	s.bclPay(cheap)
	dearer := testOrder("LINK-2", "bea@example.com") // a RM249 Pro Plan order
	dearer.formURL, dearer.subtotal = testFormURL, "249.00"
	dearer.extra = map[string]any{"amount": "250.00"} // BCL charged RM250 (RM249 and the RM1 fee); the webhook must say so too
	s.bclPay(dearer)
	if ada.me(adaToken).Pro || len(s.periodsOf("ada@example.com")) != 0 {
		t.Fatalf("a RM25 order sent with the Pro Plan's TEST form address gave something")
	}
	expectProForAYear(t, bea, beaToken)
	if periods := s.periodsOf("bea@example.com"); len(periods) != 0 {
		t.Fatalf("Bea's periods = %+v, want none: a Pro Plan order sent with CaveText Pro's address gives only CaveText Pro", periods)
	}
	_, body = s.get("/admin/log")
	expectAll(t, body, "order LINK-1: paid RM25, under the RM249 price; kept for an Admin", "order LINK-2: gave Learner")
}

const tokenRefused = "BCL API token refused"

// A revoked, expired or wrong token is named as the cause, so an Admin knows to replace it; BCL still retries.
func TestAWebhookBCLRefusesTheTokenForIsAnswered500AndLogsTheTokenAsTheCause(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
			s := confirmingSite(t, bcl)
			ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
			bcl.setRefuse(status)
			if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
				t.Fatalf("webhook with a refused token: status %d, want 500", code)
			}
			if ada.me(token).Pro {
				t.Fatalf("an order BCL could not confirm gave Pro")
			}
			_, body := s.get("/admin/log")
			expectAll(t, body, fmt.Sprintf("order LINK-1: confirm with BCL: %s (%d): replace BCL_API_TOKEN", tokenRefused, status))
			expectNone(t, body, bclToken)
		})
	}
}

func TestAWebhookWhileBCLFailsIsNotBlamedOnTheToken(t *testing.T) {
	bcl := newFakeBCL(t, map[string]bclOrder{"LINK-1": confirmed("ada@example.com")})
	s := confirmingSite(t, bcl)
	bcl.setDown(true)
	if code := s.bclPost(bclKey, bclPayload(paid("LINK-1", "ada@example.com"))); code != http.StatusInternalServerError {
		t.Fatalf("webhook while BCL is down: status %d, want 500", code)
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "order LINK-1: confirm with BCL: BCL answered 503")
	expectNone(t, body, tokenRefused)
}

func TestStartingWithATokenBCLRefusesLogsItAndStillServes(t *testing.T) {
	bcl := newFakeBCL(t, nil)
	bcl.setRefuse(http.StatusUnauthorized)
	s := confirmingSite(t, bcl) // it started: the site answers
	if got := bcl.lastAsked(t, "/forms").Header.Get("Authorization"); got != "Bearer "+bclToken {
		t.Errorf("Authorization = %q, want the token as a Bearer", got)
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, tokenRefused+" (401): replace BCL_API_TOKEN")
}

func TestStartingWhileBCLIsOutOfReachLogsItUnreachableNotRefused(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	s := confirmingSite(t, &fakeBCL{url: gone.URL})
	_, body := s.get("/admin/log")
	expectAll(t, body, "check BCL_API_TOKEN: BCL out of reach or failing")
	expectNone(t, body, tokenRefused)
}

func TestTheDailyTokenCheckLogsARefusedTokenAndNothingForAnAcceptedOne(t *testing.T) {
	bcl := newFakeBCL(t, nil)
	s := confirmingSite(t, bcl)
	s.application.CheckBCLToken(context.Background())
	_, body := s.get("/admin/log")
	expectNone(t, body, tokenRefused, "check BCL_API_TOKEN")

	bcl.setRefuse(http.StatusForbidden)
	s.application.CheckBCLToken(context.Background())
	_, body = s.get("/admin/log")
	expectAll(t, body, tokenRefused+" (403): replace BCL_API_TOKEN")
}

func TestWithoutATokenBCLIsNeverAskedAtStartup(t *testing.T) {
	bcl := newFakeBCL(t, nil)
	s := bclSite(t, func(c *config.Config) { c.BCLAPIURL = bcl.url })
	s.application.CheckBCLToken(context.Background())
	bcl.mu.Lock()
	defer bcl.mu.Unlock()
	if len(bcl.asked) != 0 {
		t.Fatalf("BCL's API was asked %d times without a token", len(bcl.asked))
	}
}
