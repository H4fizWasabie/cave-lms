package app_test

import (
	"context"
	"errors"
	"net/http"
	"regexp"
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
	expectAll(t, emails[0].Body, "Hai Bea Lim", "https://cave.example.invalid/gallery?format=ebook", "bea@example.com", "Workbook: https://cave.example.invalid/ebook-downloads/", "selama 7 hari")
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

// ebookEmailAfterPayment pays Ada's COMBO and returns her one Ebook email's body.
func ebookEmailAfterPayment(t *testing.T, s *site) string {
	t.Helper()
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	emails := withSubject(s.emailsTo("ada@example.com"), ebookEmailSubject)
	if len(emails) != 1 {
		t.Fatalf("Ada got %d Ebook emails, want 1", len(emails))
	}
	if len(emails[0].Attachments) > 0 {
		t.Fatalf("the Ebook email carries %d attachments, want none", len(emails[0].Attachments))
	}
	return emails[0].Body
}

var ebookDownloadLink = regexp.MustCompile(`(?m)^(.+): (https://cave\.example\.invalid(/ebook-downloads/[A-Za-z0-9_-]{43,}))$`)

// The Ebook email gives a download link for each ebook that is not Promotional material, with its title, and it
// downloads the file with no sign-in.
func TestTheEbookEmailLinksEachNonPromotionalEbookAndNothingElse(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t) // uploads "Workbook"
	fields := titleFields("Free sampler", "everyone")
	fields["promotional"] = "on"
	res, body := s.upload(fields, pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.uploadAs("Poster", "everyone", pngFile)

	email := ebookEmailAfterPayment(t, s)
	links := ebookDownloadLink.FindAllStringSubmatch(email, -1)
	if len(links) != 1 || links[0][1] != "Workbook" {
		t.Fatalf("the Ebook email links %v, want only Workbook: %s", links, email)
	}
	expectAll(t, email, "https://cave.example.invalid/gallery?format=ebook", "open until", "7 days")

	res, body = s.second().get(links[0][3]) // signed out
	expectStatus(t, res, body, http.StatusOK, "")
	if body != string(pdfFile) {
		t.Errorf("the link downloads %q, want the uploaded bytes", body)
	}
	if got := res.Header.Get("Content-Disposition"); got != `attachment; filename="Workbook.pdf"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control = %q, want private, no-store", got)
	}
}

// ebookLinkFor pays Ada's COMBO and returns the path of her Workbook download link.
func ebookLinkFor(t *testing.T, s *site) string {
	t.Helper()
	links := ebookDownloadLink.FindAllStringSubmatch(ebookEmailAfterPayment(t, s), -1)
	if len(links) != 1 {
		t.Fatalf("the Ebook email has %d download links, want 1", len(links))
	}
	return links[0][3]
}

// expectLinkGone checks the link answers with the page that sends the Learner to the Gallery and the Admin.
func expectLinkGone(t *testing.T, s *site, link string, status int) {
	t.Helper()
	res, body := s.second().get(link)
	expectStatus(t, res, body, status, "This download link no longer works")
	expectAll(t, body, `href="/gallery?format=ebook"`, "send you a fresh link")
	if strings.Contains(body, string(pdfFile)) || res.Header.Get("Content-Disposition") != "" {
		t.Errorf("the dead link still serves the file")
	}
}

func TestADownloadLinkStopsWorkingAfter7Days(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	link := ebookLinkFor(t, s)
	if _, err := s.db.Exec(`UPDATE ebook_download_links SET expires_at = now() + interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	res, body := s.second().get(link)
	expectStatus(t, res, body, http.StatusOK, "")
	if _, err := s.db.Exec(`UPDATE ebook_download_links SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	expectLinkGone(t, s, link, http.StatusGone)
}

func TestADownloadLinkLastsSevenDays(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	ebookLinkFor(t, s)
	var days float64
	if err := s.db.QueryRow(`SELECT extract(epoch FROM expires_at - created_at) / 86400 FROM ebook_download_links`).Scan(&days); err != nil {
		t.Fatal(err)
	}
	if days < 6.99 || days > 7.01 {
		t.Fatalf("the link lasts %v days, want 7", days)
	}
}

func TestADownloadLinkStopsWorkingWhenTheEbookAccessEnds(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	link := ebookLinkFor(t, s)
	s.endPro("ada@example.com")
	expectLinkGone(t, s, link, http.StatusGone)
}

func TestADownloadLinkStopsWorkingWhenTheEbookBecomesPromotional(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	link := ebookLinkFor(t, s)
	if _, err := s.db.Exec(`UPDATE gallery_items SET promotional = true`); err != nil {
		t.Fatal(err)
	}
	expectLinkGone(t, s, link, http.StatusGone)
}

func TestADownloadLinkOfADeletedEbookIsUnknown(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	link := ebookLinkFor(t, s)
	if _, err := s.db.Exec(`DELETE FROM gallery_items`); err != nil {
		t.Fatal(err)
	}
	expectLinkGone(t, s, link, http.StatusNotFound)
}

func TestAnUnknownDownloadLinkIsNotFound(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	expectLinkGone(t, s, "/ebook-downloads/"+strings.Repeat("a", 43), http.StatusNotFound)
}

func TestTheDownloadLinkPageIsInMalay(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	res, body := s.second().getAsking("/ebook-downloads/"+strings.Repeat("a", 43), "ms")
	expectStatus(t, res, body, http.StatusNotFound, "Pautan muat turun ini tidak lagi berfungsi")
}

func TestTooManyDownloadLinkLookupsFromOneAddressAreRefused(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	visitor := s.second()
	for range 60 {
		res, body := visitor.get("/ebook-downloads/" + strings.Repeat("a", 43))
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	res, body := visitor.get("/ebook-downloads/" + strings.Repeat("a", 43))
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
}

// With no ebook to link, the email reads as it always did.
func TestTheEbookEmailCarriesTheGalleryLinkOnlyWhenNoEbookIsUploaded(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	if _, err := s.db.Exec(`DELETE FROM gallery_items`); err != nil {
		t.Fatal(err)
	}
	body := ebookEmailAfterPayment(t, s)
	expectNone(t, body, "ebook-downloads", "7 days")
	expectAll(t, body, "https://cave.example.invalid/gallery?format=ebook", "open until")
}

// A link that cannot be made costs the email its download links, not the email.
func TestTheEbookEmailStillGoesWhenADownloadLinkCannotBeMade(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	if _, err := s.db.Exec(`DROP TABLE ebook_download_links`); err != nil {
		t.Fatal(err)
	}
	body := ebookEmailAfterPayment(t, s)
	expectNone(t, body, "ebook-downloads", "7 days")
	expectAll(t, body, "https://cave.example.invalid/gallery?format=ebook")
	_, logBody := s.get("/admin/log")
	expectAll(t, logBody, "Ebook email", "download link")
}

func TestTheDailyPruneDeletesExpiredDownloadLinks(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	ebookLinkFor(t, s)
	if _, err := s.db.Exec(`INSERT INTO ebook_download_links (learner_id, gallery_item_id, token_hash, expires_at)
		SELECT learner_id, gallery_item_id, '\x01', now() - interval '1 second' FROM ebook_download_links`); err != nil {
		t.Fatal(err)
	}
	s.application.RunDailyPrune(context.Background())
	var expired, live int
	if err := s.db.QueryRow(`SELECT count(*) FILTER (WHERE expires_at <= now()), count(*) FILTER (WHERE expires_at > now()) FROM ebook_download_links`).Scan(&expired, &live); err != nil {
		t.Fatal(err)
	}
	if expired != 0 || live != 1 {
		t.Fatalf("after the prune %d expired and %d live links, want 0 and 1", expired, live)
	}
}
