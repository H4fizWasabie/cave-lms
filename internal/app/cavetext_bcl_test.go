package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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
// CaveText Pro payment link set to proFormURL.
func bclSite(t *testing.T) *site {
	t.Helper()
	s := newSite(t, func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.CaveTextExtensionIDs = []string{extensionID}
		c.BCLWebhookKey = bclKey
	})
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return s
}

// bclPayment is the part of BCL's payment-success webhook that CAVELMS reads.
type bclPayment struct {
	event, formURL, order, payerEmail, cavelmsEmail, subtotal, kind string
	paid                                                            int
}

// paid is a paid RM25 order on the Pro form naming email in both email fields.
func paid(order, email string) bclPayment {
	return bclPayment{event: "payment-success", formURL: proFormURL, order: order, payerEmail: email, cavelmsEmail: email, subtotal: "25.00", kind: "live", paid: 1}
}

// bclPayload builds the webhook body in BCL's own shape, as captured from a Test Mode payment on 22 Oct 2026.
func bclPayload(p bclPayment) string {
	b, _ := json.Marshal(map[string]any{
		"event": p.event,
		"data": map[string]any{
			"formable_type": "payment_form", "formable_id": 19548, "form_title": "CaveText Pro (1 tahun / 1 year)",
			"form_url": p.formURL, "record_type": "Transaction", "record_id": p.order,
			"main_data": map[string]any{
				"payer_name": "Payer", "payer_email": p.payerEmail, "payer_telephone_number": "+60123456789",
				"order_number": p.order, "transaction_type": p.kind, "currency": "MYR",
				"subtotal_amount": p.subtotal, "fee_amount": "1.00", "amount": "26.00", "payment_channel": "FPX",
				"status": 3, "status_description": "Approved", "is_paid": p.paid, "source_type": "payment_form",
			},
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
