package app_test

import (
	"html"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"
)

// oprTeacher is a Teacher, Ada, on the Teacher trial, with the Admin's site, which has the five OPR styles to hide, in
// front of her; both write English.
func oprTeacher(t *testing.T) (admin, teacher *site) {
	t.Helper()
	admin = oprAdmin(t)
	teacher = newDSKPTeacher(admin)
	teacher.switchLanguage("en", "/")
	return admin, teacher
}

func TestOnlyWhoseTrialRunsOrHasProOrIsTheAdminOpensTheOPRPage(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)

	res, body := teacher.get("/teachers/opr")
	expectStatus(t, res, body, http.StatusOK, "OPR prompt")

	res, body = admin.second().get("/teachers/opr")
	expectRedirect(t, res, body, "/login?next=%2Fteachers%2Fopr")

	noProfile := admin.second()
	noProfile.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body = noProfile.get("/teachers/opr")
	expectRedirect(t, res, body, "/teachers/profile")

	res, body = admin.get("/teachers/opr") // the Admin needs no profile
	expectStatus(t, res, body, http.StatusOK, "OPR prompt")

	admin.ageTrial(14) // the trial has ended: only the pay page is left
	res, body = teacher.get("/teachers/opr")
	expectRedirect(t, res, body, "/teachers")
	admin.giveTeacherPro("ada@example.com", "month")
	res, body = teacher.get("/teachers/opr")
	expectStatus(t, res, body, http.StatusOK, "OPR prompt")
}

func TestTheOPRFormAsksForTheDetailsAndOffersOnlyShownStylesAndTheNavHasAnOPRItem(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	admin.post("/admin/opr-styles/3/hide", nil)

	_, body := teacher.get("/teachers")
	expectAll(t, body, `href="/teachers/opr"`)
	res, body := teacher.get("/teachers/opr")
	expectStatus(t, res, body, http.StatusOK, "3 of 3 OPR prompts left today")
	expectAll(t, body, `action="/teachers/opr"`, `aria-current="page"`,
		`name="title"`, `name="institution"`, `name="date"`, `name="place"`, `name="reporter"`, `name="reference"`, `name="extract"`,
		`name="time"`, `name="target"`, `name="extra"`, `name="colour"`, `name="style"`, `name="language"`)
	expectInOrder(t, body, "Activity (placeholder)", "Achievement (placeholder)", "KPI dashboard (placeholder)", "Inspection (placeholder)")
	expectNone(t, body, "Co-curricular (placeholder)", `type="file"`)
	expectAll(t, body, `<option value="en" selected>`)
	teacher.switchLanguage("ms", "/teachers/opr")
	_, body = teacher.get("/teachers/opr")
	expectAll(t, body, `<option value="ms" selected>`, "3 daripada 3 prompt OPR lagi hari ini")
}

func TestWithAllFiveOPRStylesHiddenThePageSaysOPRPromptsAreNotReadyAndOffersNoForm(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	for _, pos := range []string{"1", "2", "3", "4", "5"} {
		admin.post("/admin/opr-styles/"+pos+"/hide", nil)
	}
	res, body := teacher.get("/teachers/opr")
	expectStatus(t, res, body, http.StatusOK, "OPR prompts are not ready yet")
	expectNone(t, body, "<form class=\"form\"", `name="title"`)

	res, body = teacher.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusServiceUnavailable, "OPR prompts are not ready yet")
	if n := teacher.countOPRPrompts(); n != 0 {
		t.Errorf("%d OPR prompts were made while none were ready", n)
	}

	admin.post("/admin/opr-styles/4/show", nil)
	_, body = teacher.get("/teachers/opr")
	expectAll(t, body, `name="title"`, "KPI dashboard (placeholder)")
	expectNone(t, body, "not ready")
}

func TestTheOPRPromptCarriesTheFixedCoreInEnglishWhateverTheOPRsLanguageAndOmitsWhatWasNotTyped(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)

	_, body := teacher.post("/teachers/opr", oprForm("1")) // Malay, with no optional detail
	prompt := oprPromptOf(t, body)
	expectInOrder(t, prompt, "ONE finished, high-resolution A4 portrait infographic image in Bahasa Malaysia", "SOURCE RULE", "LOCKED DETAILS", "EXTRACT FROM THE ATTACHED REFERENCE",
		"SELECTED OPR STYLE", "CONTENT REQUIREMENTS", "VISUAL DIRECTION", "MANDATORY TWO-STAGE ANALYSIS")
	expectAll(t, prompt, "ONE-PAGE REPORT (OPR)", "ONLY it and the locked details", "exactly as written", "Do not invent", "insert placeholders",
		"two-column table containing ALL the locked details", "MAKLUMAT PROGRAM / AKTIVITI", `"Disediakan oleh: Cikgu Ada binti Ali"`, `"Sumber: laporan-hari-sukan.pdf"`,
		"Stage 1", "Stage 2", "Return only the finished OPR infographic image", "- Institution: SK Taman Maju", "- Venue: Padang SK Taman Maju",
		"Activity (placeholder)")
	expectNone(t, prompt, "Main colour", "ADDITIONAL INSTRUCTION", "- Time:", "- Target group:", "Prepared by")

	v := oprForm("2")
	v.Set("language", "en")
	_, body = teacher.post("/teachers/opr", v)
	prompt = oprPromptOf(t, body)
	expectAll(t, prompt, "infographic image in English", "PROGRAMME / ACTIVITY INFORMATION", `"Prepared by: Cikgu Ada binti Ali"`, `"Source: laporan-hari-sukan.pdf"`,
		"ONLY it and the locked details", "Achievement (placeholder)")
	expectNone(t, prompt, "Bahasa Malaysia", "MAKLUMAT PROGRAM", "Disediakan oleh")

	// A language that is neither falls back to the page's, English here.
	v.Set("language", "fr")
	_, body = teacher.post("/teachers/opr", v)
	expectAll(t, oprPromptOf(t, body), "infographic image in English")
}

func TestTheOPRPromptPageShowsItReadOnlyWithACopyButtonAndAReminderToAttachTheReference(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)
	res, body := teacher.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	expectAll(t, body, `<textarea id="opr-prompt" readonly`, `data-copy="opr-prompt"`, "Attach your reference (laporan-hari-sukan.pdf) in your chatbot", "a PDF, text or picture",
		`href="/teachers/opr"`, "CAVELMS does not keep your prompts")
	expectNone(t, body, `type="file"`)
}

func TestAMissingOrTooLongOPRDetailIsRefusedCountingNothingAndKeepingWhatWasTyped(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)
	long := func(n int) string { return strings.Repeat("x", n) }
	for _, c := range []struct{ field, value, want string }{
		{"title", "  ", "Enter the title, up to 200 characters."},
		{"title", long(201), "Enter the title, up to 200 characters."},
		{"institution", "", "Enter the institution, up to 200 characters."},
		{"institution", long(201), "Enter the institution, up to 200 characters."},
		{"date", " \n ", "Enter the date, up to 100 characters."},
		{"date", long(101), "Enter the date, up to 100 characters."},
		{"place", "", "Enter the venue, up to 200 characters."},
		{"place", long(201), "Enter the venue, up to 200 characters."},
		{"reporter", "", "Enter the reporting teacher, up to 200 characters."},
		{"reporter", long(201), "Enter the reporting teacher, up to 200 characters."},
		{"reference", "", "Enter the name of the reference, up to 200 characters."},
		{"reference", long(201), "Enter the name of the reference, up to 200 characters."},
		{"extract", "", "Enter what to take from the reference, up to 2000 characters."},
		{"extract", long(2001), "Enter what to take from the reference, up to 2000 characters."},
		{"time", long(101), "The time can be up to 100 characters."},
		{"target", long(201), "The target group can be up to 200 characters."},
		{"extra", long(1001), "The additional instruction can be up to 1000 characters."},
		{"colour", long(101), "The main colour can be up to 100 characters."},
	} {
		v := oprForm("1")
		v.Set("time", "8.00 pagi")
		v.Set(c.field, c.value)
		res, body := teacher.post("/teachers/opr", v)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, "3 of 3 OPR prompts left today", `action="/teachers/opr"`)
		expectNone(t, body, "Your OPR prompt")
	}
	v := oprForm("1")
	v.Set("reference", "")
	v.Set("colour", "merah")
	_, body := teacher.post("/teachers/opr", v)
	expectAll(t, body, `value="Hari Sukan Tahunan 2027"`, `value="12 Mac 2027"`, `value="merah"`, ">Objektif, aktiviti dan bilangan peserta</textarea>", `<option value="ms" selected>`)
	if n := teacher.countOPRPrompts(); n != 0 {
		t.Errorf("%d OPR prompts were made by forms with a mistake", n)
	}

	// The longest allowed, and only the optional ones left out, are made.
	v = oprForm("1")
	v.Set("title", long(200))
	v.Set("extract", long(2000))
	res, body := teacher.post("/teachers/opr", v)
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
}

func TestATeacherMakesAtMostThreeOPRPromptsADayApartFromRPHPromptsAndTheFourthIsToldWhenMoreCome(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	id := path.Base(admin.publishedDSKP("Sains", "Y4"))
	admin.atMYT("2026-11-16 23:30")
	_, rphFormBody := teacher.get("/teachers/dskps/" + id + "/rph")

	for _, left := range []string{"2 of 3", "1 of 3", "0 of 3"} {
		res, body := teacher.post("/teachers/opr", oprForm("1")) // the same OPR again counts too
		expectStatus(t, res, body, http.StatusOK, left+" OPR prompts left today")
	}
	res, body := teacher.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusTooManyRequests, "You have made 3 OPR prompts today, the most a day. You can make more from midnight, Malaysia time: 17 November 2026.")
	expectAll(t, body, "0 of 3 OPR prompts left today", `value="Hari Sukan Tahunan 2027"`)
	expectNone(t, body, "Your OPR prompt", "ONE-PAGE REPORT")
	teacher.switchLanguage("ms", "/teachers/opr")
	_, body = teacher.post("/teachers/opr", oprForm("1"))
	teacher.switchLanguage("en", "/teachers/opr")
	expectAll(t, body, "mulai tengah malam, waktu Malaysia: 17 November 2026")
	if n := teacher.countOPRPrompts(); n != 3 {
		t.Errorf("%d OPR prompts are counted, want the 3 made", n)
	}

	// RPH prompts still work, with their own 3: and they do not use up OPR prompts either.
	_, body = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, "3 of 3 RPH prompts left today")
	for _, left := range []string{"2 of 3", "1 of 3", "0 of 3"} {
		res, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, rphFormBody, "1.1.1"))
		expectStatus(t, res, body, http.StatusOK, left+" RPH prompts left today")
	}

	// Looking again, and copying (which is the page's own button), change nothing; no OPR prompt made is shown: no history.
	for _, p := range []string{"/teachers/opr", "/teachers", "/teachers/dskps"} {
		_, body = teacher.get(p)
		expectNone(t, body, "ONE-PAGE REPORT", "Hari Sukan Tahunan 2027", "readonly")
	}
	_, body = teacher.get("/teachers/opr")
	expectAll(t, body, "0 of 3 OPR prompts left today")
	if n := teacher.countOPRPrompts(); n != 3 {
		t.Errorf("%d OPR prompts are counted after looking, want 3", n)
	}

	// The limit is the account's: another Teacher has her own 3.
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	bob.saveTeacherProfile("Sains", "Y4")
	bob.switchLanguage("en", "/")
	res, body = bob.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusOK, "2 of 3 OPR prompts left today")
}

func TestTheOPRDailyPromptLimitResetsAtMalaysiaMidnightNotUTCMidnight(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	make := func(at string) (int, string) {
		admin.atMYT(at)
		res, body := teacher.post("/teachers/opr", oprForm("1"))
		return res.StatusCode, body
	}
	// 00:30 on 16 November in Malaysia is 16:30 on 15 November in UTC: three made then fill 16 November, and 16:00 UTC on
	// the 16th (00:00 on the 17th in Malaysia) is the reset, not UTC midnight.
	for range 3 {
		if status, body := make("2026-11-16 00:30"); status != http.StatusOK {
			t.Fatalf("an OPR prompt at 00:30 was %d, want 200: %s", status, body)
		}
	}
	for _, at := range []string{"2026-11-16 00:31", "2026-11-16 07:59", "2026-11-16 08:00", "2026-11-16 12:00", "2026-11-16 23:59"} {
		if status, _ := make(at); status != http.StatusTooManyRequests {
			t.Errorf("an OPR prompt at %s Malaysia time was %d, want 429: still the same day", at, status)
		}
	}
	admin.atMYT("2026-11-16 23:59")
	_, body := teacher.get("/teachers/opr")
	expectAll(t, body, "0 of 3 OPR prompts left today")

	status, body := make("2026-11-17 00:00") // Malaysia midnight, which is 16:00 UTC on the 16th
	if status != http.StatusOK {
		t.Fatalf("an OPR prompt at Malaysia midnight was %d, want 200: %s", status, body)
	}
	expectAll(t, body, "2 of 3 OPR prompts left today")
}

func TestAHiddenOrUnknownOPRStylePostedIsRefusedCountingNothingAndKeepingWhatWasTyped(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	admin.post("/admin/opr-styles/3/hide", nil)
	for _, style := range []string{"3", "6", "0", "-1", "kpi", ""} {
		res, body := teacher.post("/teachers/opr", oprForm(style))
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Pick one of the OPR styles offered.")
		expectAll(t, body, `value="Hari Sukan Tahunan 2027"`, `value="SK Taman Maju"`, ">Objektif, aktiviti dan bilangan peserta</textarea>", "3 of 3 OPR prompts left today")
		expectNone(t, body, "Your OPR prompt", "Co-curricular (placeholder)")
	}
	if n := teacher.countOPRPrompts(); n != 0 {
		t.Errorf("%d OPR prompts were made with a style that was not offered", n)
	}
	res, body := teacher.post("/teachers/opr", oprForm("4"))
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
}

// oprPromptOf is the text of the readonly field holding the OPR prompt made.
func oprPromptOf(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<textarea id="opr-prompt" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no OPR prompt in the page:\n%s", body)
	}
	return html.UnescapeString(m[1])
}

func TestAnOPRPromptHoldsEveryDetailTypedExactlyAndTheChosenStylesText(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	admin.saveOPRStyle("2", "Kejohanan", "Use a big trophy layout with a podium.")

	v := oprForm("2")
	v.Set("time", "8.00 pagi hingga 1.00 petang")
	v.Set("target", "Murid Tahun 4 hingga 6")
	v.Set("extra", "Letakkan logo sekolah di penjuru kiri.")
	v.Set("colour", "biru tua #1F3A68")
	res, body := teacher.post("/teachers/opr", v)
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	expectAll(t, body, "2 of 3 OPR prompts left today")
	prompt := oprPromptOf(t, body)
	expectAll(t, prompt, "Hari Sukan Tahunan 2027", "SK Taman Maju", "12 Mac 2027", "8.00 pagi hingga 1.00 petang", "Padang SK Taman Maju",
		"Murid Tahun 4 hingga 6", "Cikgu Ada binti Ali", "laporan-hari-sukan.pdf", "Objektif, aktiviti dan bilangan peserta",
		"Use a big trophy layout with a podium.", "Letakkan logo sekolah di penjuru kiri.", "biru tua #1F3A68")
	expectNone(t, prompt, "Activity (placeholder)", "[PLACEHOLDER")
	if n := teacher.countOPRPrompts(); n != 1 {
		t.Errorf("%d OPR prompts are counted, want the 1 made", n)
	}
}

// oprForm is an OPR's details as a Teacher types them, with the OPR style in slot style.
func oprForm(style string) url.Values {
	return url.Values{
		"title": {"Hari Sukan Tahunan 2027"}, "institution": {"SK Taman Maju"}, "date": {"12 Mac 2027"}, "place": {"Padang SK Taman Maju"},
		"reporter": {"Cikgu Ada binti Ali"}, "reference": {"laporan-hari-sukan.pdf"}, "extract": {"Objektif, aktiviti dan bilangan peserta"},
		"style": {style}, "language": {"ms"},
	}
}

func TestTheLastInstitusiAndGuruPelaporTypedAreStoredOnlyWhenAnOPRPromptIsMadeAndPrefilledNextTime(t *testing.T) {
	t.Parallel()
	admin, teacher := oprTeacher(t)
	_, form := teacher.get("/teachers/opr")
	expectNone(t, form, `value="SK Taman Maju"`, `value="Cikgu Ada binti Ali"`)

	// A form with a mistake makes no prompt and is not remembered.
	bad := oprForm("1")
	bad.Set("institution", "SK Bukit Indah")
	bad.Set("title", "")
	res, body := teacher.post("/teachers/opr", bad)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	_, form = teacher.get("/teachers/opr")
	expectNone(t, form, "SK Bukit Indah")

	res, body = teacher.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	_, form = teacher.get("/teachers/opr")
	expectAll(t, form, `name="institution" value="SK Taman Maju"`, `name="reporter" value="Cikgu Ada binti Ali"`)
	expectNone(t, form, "Hari Sukan Tahunan 2027") // the rest is for one OPR, not kept

	// The latest made wins, and it is the Teacher's own: another's form is still empty.
	later := oprForm("1")
	later.Set("institution", "SK Bukit Indah")
	later.Set("reporter", "Cikgu Bob")
	teacher.post("/teachers/opr", later)
	_, form = teacher.get("/teachers/opr")
	expectAll(t, form, `name="institution" value="SK Bukit Indah"`, `name="reporter" value="Cikgu Bob"`)
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	bob.saveTeacherProfile("Sains", "Y4")
	_, form = bob.get("/teachers/opr")
	expectNone(t, form, "SK Bukit Indah", "Cikgu Bob")

	// The Admin has no Teacher profile: making a prompt works and nothing is kept.
	res, body = admin.post("/teachers/opr", oprForm("1"))
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	_, form = admin.get("/teachers/opr")
	expectNone(t, form, `value="SK Taman Maju"`)
}

func TestThePrivacyNoticeSaysTheTeacherProfileKeepsTheInstitutionAndReportingTeacherLastTypedForAnOPR(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/privacy", "en")
	expectAll(t, body, "Your Teacher profile keeps the institution and reporting teacher you last typed for an OPR, to fill them in next time.")
	_, body = s.getAsking("/privacy", "ms")
	expectAll(t, body, "Profil guru anda menyimpan institusi dan guru pelapor yang terakhir anda taip untuk OPR, untuk diisi pada kali berikutnya.")
}

// countOPRPrompts is how many OPR prompts the site has recorded as made.
func (s *site) countOPRPrompts() (n int) {
	s.t.Helper()
	if err := s.db.QueryRow(`SELECT count(*) FROM prompts_made WHERE kind = 'opr'`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}
