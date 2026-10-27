package app_test

import (
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"
)

const (
	ebookEmailSubject   = "Your COMBO Pro Plan ebooks"
	ebookEmailSubjectMS = "Ebook Pelan Pro COMBO anda"
)

// ebookEmails lists the Ebook emails sent to the address, in either language.
func (s *site) ebookEmails(to string) int {
	return len(withSubject(s.emailsTo(to), ebookEmailSubject)) + len(withSubject(s.emailsTo(to), ebookEmailSubjectMS))
}

func TestAComboPaymentEmailsTheEbookLinkInTheLearnersLanguage(t *testing.T) {
	s, _ := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/") // saved on Ada's account
	bea, _ := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	bea.switchLanguage("ms", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00")) // CAVE99
	s.bclPay(couponProPlan("LINK-2", "bea@example.com", "249.00", "150.00"))

	emails := withSubject(s.emailsTo("ada@example.com"), ebookEmailSubject)
	if len(emails) != 1 {
		t.Fatalf("Ada got %d Ebook emails, want 1", len(emails))
	}
	until := time.Now().UTC().AddDate(1, 0, 0).Format("2 January 2006")
	expectAll(t, emails[0].Body, "Hi Ada Lovelace", "open until "+until, "https://cave.example.invalid/gallery?format=ebook", "Log in with this email (ada@example.com)")

	emails = withSubject(s.emailsTo("bea@example.com"), ebookEmailSubjectMS)
	if len(emails) != 1 {
		t.Fatalf("Bea got %d Ebook emails in Malay, want 1", len(emails))
	}
	expectAll(t, emails[0].Body, "Hai Bea Lim", "https://cave.example.invalid/gallery?format=ebook", "bea@example.com")
}

// A Learner who never chose a language gets the default one, as for the Certificate email.
func TestTheEbookEmailFallsBackToTheDefaultLanguage(t *testing.T) {
	s, _ := ebookSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	if n := len(withSubject(s.emailsTo("ada@example.com"), ebookEmailSubjectMS)); n != 1 {
		t.Fatalf("Ada got %d Ebook emails in the default language, want 1", n)
	}
}

func TestOtherPaymentsAndGivesSendNoEbookEmail(t *testing.T) {
	s, _ := ebookSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	full := paidProPlan("LINK-1", "ada@example.com")
	full.subtotal = "249.00" // the Pro Plan at full price: no code
	s.bclPay(full)
	s.givePro("bea@example.com")
	res, body := s.post(s.proAction("bea@example.com", "pro-plan"), nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for _, email := range []string{"ada@example.com", "bea@example.com"} {
		if n := s.ebookEmails(email); n != 0 {
			t.Errorf("%s got %d Ebook emails, want none", email, n)
		}
	}
}

// Each COMBO the Admin gives sends one, a renewal too.
func TestTheAdminsComboSendsAnEbookEmailEachTime(t *testing.T) {
	s, _ := ebookSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	for want := 1; want <= 2; want++ {
		res, body := s.post(s.proAction("ada@example.com", "combo"), nil)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		if n := s.ebookEmails("ada@example.com"); n != want {
			t.Fatalf("after %d COMBOs Ada got %d Ebook emails, want %d", want, n, want)
		}
	}
}

// A failed send never undoes the COMBO or the payment: the System log has the error.
func TestAFailedEbookEmailIsLoggedAndKeepsTheCombo(t *testing.T) {
	s, ebook := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.mail.FailWith(errors.New("mail server down"))
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	expectEbookOpens(t, ada, ebook, "inline")
	res, body := s.post(s.proAction("bea@example.com", "combo"), nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.mail.FailWith(nil)
	bea := s.second()
	bea.login("bea@example.com", "correct horse battery")
	expectEbookOpens(t, bea, ebook, "inline")

	_, body = s.get("/admin/log")
	expectAll(t, body, "send Ebook email", "mail server down")
}

// The Admin resends the link from the CaveText Learners page, for a Learner whose Ebook access is running.
func TestTheAdminResendsTheEbookLinkOnlyWhileTheAccessRuns(t *testing.T) {
	s, _ := ebookSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	button := regexp.MustCompile(`action="/admin/cavetext/learners/\d+/ebook-email"`)
	_, body := s.get("/admin/cavetext")
	expectNone(t, body, "Email ebook link")
	s.givePro("bea@example.com") // Pro without Ebooks

	res, body := s.post(s.proAction("ada@example.com", "combo"), nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/cavetext")
	if n := len(button.FindAllString(body, -1)); n != 1 {
		t.Fatalf("%d Email ebook link buttons, want 1 for Ada only: %s", n, body)
	}
	expectAll(t, body, "Email ebook link")
	res, body = s.post(s.proAction("ada@example.com", "ebook-email"), nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := s.ebookEmails("ada@example.com"); n != 2 {
		t.Fatalf("Ada got %d Ebook emails after the resend, want 2", n)
	}

	s.endPro("ada@example.com")
	_, body = s.get("/admin/cavetext")
	expectNone(t, body, "Email ebook link")
}
