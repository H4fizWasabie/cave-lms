package app_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	t.Parallel()
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
	t.Parallel()
	s, _ := ebookSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	if n := len(withSubject(s.emailsTo("ada@example.com"), ebookEmailSubjectMS)); n != 1 {
		t.Fatalf("Ada got %d Ebook emails in the default language, want 1", n)
	}
}

func TestOtherPaymentsAndGivesSendNoEbookEmail(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// ebookEmailAfterPayment pays Ada's COMBO and returns her one Ebook email's attachment count and body, and its first
// attachment's content and name.
func ebookEmailAfterPayment(t *testing.T, s *site) (attachments int, body string, first []byte, name string) {
	t.Helper()
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	emails := withSubject(s.emailsTo("ada@example.com"), ebookEmailSubject)
	if len(emails) != 1 {
		t.Fatalf("Ada got %d Ebook emails, want 1", len(emails))
	}
	email := emails[0]
	if len(email.Attachments) > 0 {
		first, name = email.Attachments[0].Content, email.Attachments[0].Filename
	}
	return len(email.Attachments), email.Body, first, name
}

const ebookAttachedLine = "A copy of each ebook is attached for you to keep."

// The Ebook email carries a copy of each ebook that is not Promotional material, and still links to the Gallery.
func TestTheEbookEmailAttachesEachNonPromotionalEbookAndNothingElse(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t) // uploads "Workbook"
	fields := titleFields("Free sampler", "everyone")
	fields["promotional"] = "on"
	res, body := s.upload(fields, pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.uploadAs("Poster", "everyone", pngFile)

	n, body, content, name := ebookEmailAfterPayment(t, s)
	if n != 1 || string(content) != string(pdfFile) || name != "Workbook.pdf" {
		t.Fatalf("the Ebook email carries %d attachments, first %q = %q; want only Workbook.pdf with the uploaded bytes", n, name, content)
	}
	expectAll(t, body, ebookAttachedLine, "https://cave.example.invalid/gallery?format=ebook", "open until")
}

// With no ebook to attach, the email reads as it always did.
func TestTheEbookEmailCarriesTheLinkOnlyWhenNoEbookIsUploaded(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	if _, err := s.db.Exec(`DELETE FROM gallery_items`); err != nil {
		t.Fatal(err)
	}
	n, body, _, _ := ebookEmailAfterPayment(t, s)
	if n != 0 {
		t.Fatalf("the Ebook email carries %d attachments, want none", n)
	}
	expectNone(t, body, ebookAttachedLine)
	expectAll(t, body, "https://cave.example.invalid/gallery?format=ebook", "open until")
}

// Above 18 MB of ebooks together the email carries the link only, and the System log says why.
func TestTheEbookEmailCarriesTheLinkOnlyWhenTheEbooksAreTooLarge(t *testing.T) {
	t.Parallel()
	s, ebook := ebookSite(t)
	id := strings.TrimPrefix(ebook, "/gallery/")
	if _, err := s.db.Exec(`UPDATE gallery_items SET size_bytes = $1 WHERE id = $2`, 18*1024*1024+1, id); err != nil {
		t.Fatal(err)
	}

	n, body, _, _ := ebookEmailAfterPayment(t, s)
	if n != 0 {
		t.Fatalf("the Ebook email carries %d attachments, want none", n)
	}
	expectNone(t, body, ebookAttachedLine)
	expectAll(t, body, "https://cave.example.invalid/gallery?format=ebook", "open until")
	_, logBody := s.get("/admin/log")
	expectAll(t, logBody, "Ebook email", strconv.Itoa(18*1024*1024+1)+" bytes", "link only")
}

// An ebook whose file cannot be read costs the email its attachments, not the email.
func TestTheEbookEmailStillGoesWhenAnEbookFileCannotBeRead(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	var key string
	if err := s.db.QueryRow(`SELECT storage_key FROM gallery_items WHERE format = 'ebook'`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.storageDir, key)); err != nil {
		t.Fatal(err)
	}

	n, body, _, _ := ebookEmailAfterPayment(t, s)
	if n != 0 {
		t.Fatalf("the Ebook email carries %d attachments, want none", n)
	}
	expectNone(t, body, ebookAttachedLine)
	expectAll(t, body, "https://cave.example.invalid/gallery?format=ebook")
	_, logBody := s.get("/admin/log")
	expectAll(t, logBody, "Ebook email")
}
