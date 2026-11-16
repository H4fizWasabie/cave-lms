package app_test

import (
	"html"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// rphTeacher is a Teacher, Ada, with a published Malay DSKP of Sains Year 4 in front of her: the Admin's site, her own, and
// the DSKP's id.
func rphTeacher(t *testing.T) (admin, teacher *site, id string) {
	t.Helper()
	admin = newDSKPAdmin(t)
	id = path.Base(admin.publishedDSKP("Sains", "Y4"))
	return admin, newDSKPTeacher(admin), id
}

var spValue = regexp.MustCompile(`name="sp" value="(\d+)"[^>]*> <strong>SP ([\d.]+)</strong>`)

// spIDs reads the form's SP check boxes: the value each SP code posts.
func spIDs(t *testing.T, body string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, m := range spValue.FindAllStringSubmatch(body, -1) {
		ids[m[2]] = m[1]
	}
	return ids
}

// rphForm is a lesson as a Teacher types it, with the SPs ticked by code.
func rphForm(t *testing.T, form string, sps ...string) url.Values {
	t.Helper()
	ids := spIDs(t, form)
	v := url.Values{"date": {"2027-02-03"}, "time": {"8.00 - 9.00 pagi"}, "class": {"4 Bestari"}, "students": {"31"}, "notes": {"PAK21, BBM: kad gambar"}}
	for _, code := range sps {
		if ids[code] == "" {
			t.Fatalf("no check box for SP %s in %s", code, form)
		}
		v.Add("sp", ids[code])
	}
	return v
}

func TestATeacherSeesTheStandardsOfAPublishedDSKPToTickAndTheLessonDetailsToType(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)

	_, body := teacher.get("/teachers/dskps/" + id)
	expectAll(t, body, `href="/teachers/dskps/`+id+`/rph"`)
	res, body := teacher.get("/teachers/dskps/" + id + "/rph")
	expectStatus(t, res, body, http.StatusOK, "Sains")
	expectAll(t, body, `action="/teachers/dskps/`+id+`/rph"`, "Tema 1: Kajian Saintifik", "SK 1.1", "SK 1.2", "Memahami kemahiran saintifik",
		"SP 1.1.1", "Mengenal pasti kemahiran proses sains", "SP 1.1.2", "SP 1.2.1",
		`name="date"`, `name="time"`, `name="class"`, `name="students"`, `name="notes"`, `name="language"`,
		"3 of 3 RPH prompts left today")
	if got := spIDs(t, body); len(got) != 3 {
		t.Errorf("the form has check boxes for %v, want the 3 SPs", got)
	}
}

func TestThePortalsHomeLeadsToTheDSKPsToMakeAnRPHPromptFrom(t *testing.T) {
	t.Parallel()
	_, teacher, _ := rphTeacher(t)
	_, body := teacher.get("/teachers")
	expectAll(t, body, `href="/teachers/dskps"`, "Pick a DSKP and tick the standards for your lesson to make an RPH prompt.")
}

// A Teacher's own upload, read and unchecked, makes RPH prompts like a published DSKP; nobody else's does, and neither does
// one still being read or failed.
func TestATeacherMakesRPHPromptsFromTheirOwnUploadedDSKPAndNoOnesElse(t *testing.T) {
	t.Parallel()
	admin := newDSKPAdmin(t)
	ada := newProDSKPTeacher(admin)
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	bob.saveTeacherProfile("Sejarah", "Y4")

	release := admin.ai.HoldDSKPs()
	res, body := ada.uploadOwnDSKP("Sejarah", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	own := path.Base(res.Header.Get("Location"))
	res, body = ada.get("/teachers/dskps/" + own + "/rph") // still being read: no standards to tick
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = ada.get("/teachers/dskps/" + own)
	expectNone(t, body, "/rph")
	release()
	admin.application.WaitForDSKPReads()

	_, body = ada.get("/teachers/dskps/" + own)
	expectAll(t, body, `href="/teachers/dskps/`+own+`/rph"`)
	res, form := ada.get("/teachers/dskps/" + own + "/rph")
	expectStatus(t, res, form, http.StatusOK, "Sejarah")
	res, body = ada.post("/teachers/dskps/"+own+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	expectAll(t, promptOf(t, body), "Mata pelajaran: Sejarah", "Standard Pembelajaran (SP) 1.1.1")

	res, body = bob.get("/teachers/dskps/" + own + "/rph")
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = bob.post("/teachers/dskps/"+own+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = admin.get("/teachers/dskps/" + own + "/rph")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestAnRPHPromptIsBuiltFromTheTickedStandardsAndTheLessonDetailsWithNoAICall(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	asked := len(admin.ai.DSKPRequests())
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")

	res, body := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1", "1.1.2"))
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	prompt := promptOf(t, body)
	expectAll(t, prompt, "Rancangan Pelajaran Harian (RPH)", "Mata pelajaran: Sains", "Tahun: Tahun 4", "Tarikh: 3 Februari 2027", "Masa atau tempoh: 8.00 - 9.00 pagi",
		"Kelas: 4 Bestari", "Bilangan murid: 31", "Tema atau bidang: Tema 1: Kajian Saintifik", "Tajuk: Kemahiran Saintifik",
		"Standard Kandungan (SK) 1.1: Memahami kemahiran saintifik", "Standard Pembelajaran (SP) 1.1.1: Mengenal pasti kemahiran proses sains",
		"Standard Pembelajaran (SP) 1.1.2: Menyatakan kemahiran manipulatif", "PAK21, BBM: kad gambar",
		"Objektif", "Kriteria kejayaan", "Aktiviti", "set induksi", "perkembangan", "penutup", "Bahan bantu mengajar (BBM)", "Penilaian", "Refleksi", "Bahasa Melayu")
	expectNone(t, prompt, "1.2.1", "Membuat hipotesis", "SK 1.2")
	expectInOrder(t, prompt, "Objektif", "Kriteria kejayaan", "Aktiviti", "Bahan bantu mengajar", "Penilaian", "Refleksi")
	expectAll(t, body, "2 of 3 RPH prompts left today", `data-copy="rph-prompt"`)
	if got := len(admin.ai.DSKPRequests()); got != asked {
		t.Errorf("making an RPH prompt asked the AI: %d requests, was %d", got, asked)
	}
}

func TestThePromptIsInTheDSKPsLanguageUnlessTheTeacherSwitchesIt(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	res, body := admin.uploadDSKP("Science", "Y5", "en", "science.pdf", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.application.WaitForDSKPReads()
	english := path.Base(res.Header.Get("Location"))
	admin.post("/admin/dskps/"+english+"/publish", nil)

	// A Malay DSKP's form asks for Malay, and the prompt is in Malay with its English twin beside it.
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, form, `<option value="ms" selected>`)
	_, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectAll(t, promptOf(t, body), "Mata pelajaran: Sains", "Tulis RPH dalam Bahasa Melayu")
	expectAll(t, body, "The same prompt in English", `id="rph-prompt-other"`, `data-copy="rph-prompt-other"`)
	other := regexp.MustCompile(`(?s)<textarea id="rph-prompt-other" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)[1]
	expectAll(t, other, "Subject: Sains", "Write the RPH in English", "Success criteria", "Learning Standard (Standard Pembelajaran, SP) 1.1.1: Mengenal pasti kemahiran proses sains")
	expectNone(t, other, "Mata pelajaran", "Tulis RPH dalam")

	// Switching it makes the English prompt the main one, and the Malay its twin.
	switched := rphForm(t, form, "1.1.1")
	switched.Set("language", "en")
	_, body = teacher.post("/teachers/dskps/"+id+"/rph", switched)
	expectAll(t, promptOf(t, body), "Subject: Sains", "Write the RPH in English", "Date: 3 February 2027", "Year: Year 4")
	expectAll(t, body, "The same prompt in Malay")

	// An English DSKP's form asks for English. (The next day, so the earlier prompts do not count.)
	admin.atMYT("2026-11-18 09:00")
	_, form = teacher.get("/teachers/dskps/" + english + "/rph")
	expectAll(t, form, `<option value="en" selected>`)
	_, body = teacher.post("/teachers/dskps/"+english+"/rph", rphForm(t, form, "1.1.1"))
	expectAll(t, promptOf(t, body), "Subject: Science", "Write the RPH in English")

	// The page's own language does not change the prompt's.
	ms := rphForm(t, form, "1.1.1")
	_, body = teacher.postAsking("/teachers/dskps/"+english+"/rph", "ms", ms)
	expectAll(t, promptOf(t, body), "Subject: Science", "Write the RPH in English")
	expectAll(t, body, "Prompt RPH anda", "Prompt yang sama dalam Bahasa Melayu")
}

func TestAFormWithAMistakeMakesNoPromptCountsNoneAndComesBackAsTyped(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	other := path.Base(admin.publishedDSKP("Matematik", "Y4"))
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	_, otherForm := teacher.get("/teachers/dskps/" + other + "/rph")

	with := func(v url.Values, key, value string) url.Values { v.Set(key, value); return v }
	for _, c := range []struct {
		name, want string
		form       url.Values
	}{
		{"no SP ticked", "Tick at least one SP.", rphForm(t, form)},
		{"SPs under two SKs", "Tick SPs under one SK only.", rphForm(t, form, "1.1.1", "1.2.1")},
		{"an SP of another DSKP", "Tick at least one SP.", rphForm(t, otherForm, "1.1.1")},
		{"an SP that is not there", "Tick at least one SP.", with(rphForm(t, form), "sp", "999999")},
		{"a date that is not one", "Enter the date of the lesson.", with(rphForm(t, form, "1.1.1"), "date", "3/2/2027")},
		{"no date", "Enter the date of the lesson.", with(rphForm(t, form, "1.1.1"), "date", "")},
		{"no time", "Enter the time or duration", with(rphForm(t, form, "1.1.1"), "time", "  ")},
		{"a long time", "Enter the time or duration", with(rphForm(t, form, "1.1.1"), "time", strings.Repeat("x", 61))},
		{"no class", "Enter the class", with(rphForm(t, form, "1.1.1"), "class", "")},
		{"no murid", "Enter the number of pupils, from 1 to 200.", with(rphForm(t, form, "1.1.1"), "students", "")},
		{"no murid at all", "Enter the number of pupils, from 1 to 200.", with(rphForm(t, form, "1.1.1"), "students", "0")},
		{"too many murid", "Enter the number of pupils, from 1 to 200.", with(rphForm(t, form, "1.1.1"), "students", "201")},
		{"murid in words", "Enter the number of pupils, from 1 to 200.", with(rphForm(t, form, "1.1.1"), "students", "tiga puluh")},
		{"long notes", "Notes can be up to 1000 characters.", with(rphForm(t, form, "1.1.1"), "notes", strings.Repeat("x", 1001))},
	} {
		res, body := teacher.post("/teachers/dskps/"+id+"/rph", c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, "3 of 3 RPH prompts left today", `action="/teachers/dskps/`+id+`/rph"`)
		expectNone(t, body, "Your RPH prompt")
	}

	// The form comes back as it was typed and ticked.
	bad := with(rphForm(t, form, "1.1.2"), "students", "")
	_, body := teacher.post("/teachers/dskps/"+id+"/rph", bad)
	expectAll(t, body, `value="4 Bestari"`, `value="8.00 - 9.00 pagi"`, `value="2027-02-03"`, ">PAK21, BBM: kad gambar</textarea>")
	if got := regexp.MustCompile(`name="sp" value="\d+" checked>`).FindAllString(body, -1); len(got) != 1 {
		t.Errorf("%d SPs are ticked again, want the 1 ticked", len(got))
	}
	if n := teacher.countRPHPrompts(); n != 0 {
		t.Errorf("%d prompts were made by forms with a mistake", n)
	}
}

// myt is a moment in Malaysia time.
func myt(t *testing.T, at string) time.Time {
	t.Helper()
	moment, err := time.ParseInLocation("2006-01-02 15:04", at, time.FixedZone("MYT", 8*60*60))
	if err != nil {
		t.Fatal(err)
	}
	return moment
}

// atMYT sets the site clock to a moment in Malaysia time.
func (s *site) atMYT(at string) {
	s.t.Helper()
	moment := myt(s.t, at)
	s.application.SetClock(func() time.Time { return moment })
}

func TestATeacherMakesAtMostThreeRPHPromptsADayAndTheFourthIsToldWhenMoreCome(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	admin.atMYT("2026-11-16 23:30")
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")

	for i, left := range []string{"2 of 3", "1 of 3", "0 of 3"} {
		res, body := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1")) // the same lesson again counts too
		expectStatus(t, res, body, http.StatusOK, left+" RPH prompts left today")
		if i == 2 {
			expectAll(t, body, "0 of 3 RPH prompts left today")
		}
	}
	res, body := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusTooManyRequests, "You have made 3 RPH prompts today, the most a day. You can make more from midnight, Malaysia time: 17 November 2026.")
	expectNone(t, body, "Your RPH prompt", "Rancangan Pelajaran Harian")
	_, body = teacher.postAsking("/teachers/dskps/"+id+"/rph", "ms", rphForm(t, form, "1.1.1"))
	expectAll(t, body, "mulai tengah malam, waktu Malaysia: 17 November 2026")
	if n := teacher.countRPHPrompts(); n != 3 {
		t.Errorf("%d prompts are counted, want the 3 made", n)
	}

	// Looking again changes nothing, and nothing of the prompts made is shown: no history.
	_, body = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, "0 of 3 RPH prompts left today")
	expectNone(t, body, "Rancangan Pelajaran Harian", "4 Bestari", "readonly")
	for _, p := range []string{"/teachers", "/teachers/dskps", "/teachers/dskps/" + id} {
		_, body = teacher.get(p)
		expectNone(t, body, "Rancangan Pelajaran Harian", "4 Bestari")
	}
	if n := teacher.countRPHPrompts(); n != 3 {
		t.Errorf("%d prompts are counted after looking, want 3", n)
	}

	// The limit is the account's: another Teacher has her own 3.
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	bob.saveTeacherProfile("Sains", "Y4")
	res, body = bob.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusOK, "2 of 3 RPH prompts left today")
}

func TestTheDailyPromptLimitResetsAtMalaysiaMidnightNotUTCMidnight(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	make := func(at string) (int, string) {
		admin.atMYT(at)
		res, body := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
		return res.StatusCode, body
	}
	// 00:30 on 16 November in Malaysia is 16:30 on 15 November in UTC: three made then fill 16 November, and 16:00 UTC
	// on the 16th (00:00 on the 17th in Malaysia) is the reset, not UTC midnight.
	for range 3 {
		if status, body := make("2026-11-16 00:30"); status != http.StatusOK {
			t.Fatalf("a prompt at 00:30 was %d, want 200: %s", status, body)
		}
	}
	for _, at := range []string{"2026-11-16 00:31", "2026-11-16 07:59", "2026-11-16 08:00", "2026-11-16 12:00", "2026-11-16 23:59"} {
		if status, _ := make(at); status != http.StatusTooManyRequests {
			t.Errorf("a prompt at %s Malaysia time was %d, want 429: still the same day", at, status)
		}
	}
	admin.atMYT("2026-11-16 23:59")
	_, body := teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, "0 of 3 RPH prompts left today")

	status, body := make("2026-11-17 00:00") // Malaysia midnight, which is 16:00 UTC on the 16th
	if status != http.StatusOK {
		t.Fatalf("a prompt at Malaysia midnight was %d, want 200: %s", status, body)
	}
	expectAll(t, body, "2 of 3 RPH prompts left today")
	// The day before's prompts do not count against the new day, which has its own 3.
	for range 2 {
		if status, _ := make("2026-11-17 09:00"); status != http.StatusOK {
			t.Fatalf("a prompt on the next day was %d, want 200", status)
		}
	}
	if status, _ := make("2026-11-17 23:59"); status != http.StatusTooManyRequests {
		t.Errorf("the next day's fourth prompt was %d, want 429", status)
	}
}

func TestPromptsMadeAtOnceCannotPassTheDailyLimit(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 8 {
		wg.Go(func() {
			if res, _ := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1")); res.StatusCode == http.StatusOK {
				ok.Add(1)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 3 || teacher.countRPHPrompts() != 3 {
		t.Errorf("%d of 8 prompts made at once were given, %d counted; want 3", ok.Load(), teacher.countRPHPrompts())
	}
}

func TestOnlyWhoseTrialRunsOrIsTheAdminMakesRPHPromptsFromPublishedDSKPsOnly(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	draft := path.Base(admin.uploadedDSKP("Sejarah", "F1"))
	admin.application.WaitForDSKPReads()

	visitor := admin.second()
	res, body := visitor.get("/teachers/dskps/" + id + "/rph")
	expectRedirect(t, res, body, "/login?next=%2Fteachers%2Fdskps%2F"+id+"%2Frph")
	res, body = visitor.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectRedirect(t, res, body, "/login")

	noProfile := admin.second()
	noProfile.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body = noProfile.get("/teachers/dskps/" + id + "/rph")
	expectRedirect(t, res, body, "/teachers/profile")
	res, body = noProfile.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectRedirect(t, res, body, "/teachers/profile")

	// A DSKP still a draft, one that is not there and an id that is not a number are not found.
	for _, other := range []string{draft, "999", "abc"} {
		res, body = teacher.get("/teachers/dskps/" + other + "/rph")
		if other == "abc" || other == "999" || other == draft {
			expectStatus(t, res, body, http.StatusNotFound, "")
		}
		res, body = teacher.post("/teachers/dskps/"+other+"/rph", rphForm(t, form, "1.1.1"))
		expectStatus(t, res, body, http.StatusNotFound, "")
	}

	// The Admin passes without a profile.
	res, body = admin.get("/teachers/dskps/" + id + "/rph")
	expectStatus(t, res, body, http.StatusOK, "3 of 3 RPH prompts left today")
	res, body = admin.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusOK, "2 of 3 RPH prompts left today")

	// Once the trial has ended only the pay page is left, and no prompt is made.
	admin.ageTrial(14)
	res, body = teacher.get("/teachers/dskps/" + id + "/rph")
	expectRedirect(t, res, body, "/teachers")
	res, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectRedirect(t, res, body, "/teachers")
	if n := teacher.countRPHPrompts(); n != 1 {
		t.Errorf("%d prompts are counted, want the Admin's 1", n)
	}
}

func TestThePromptAsksTheChatbotForEverySectionOfAnRPHInBothLanguagesAndCopyingIsOneClick(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	v := rphForm(t, form, "1.2.1")
	v.Set("language", "en")
	_, body := teacher.post("/teachers/dskps/"+id+"/rph", v)

	expectAll(t, promptOf(t, body), "Objective", "Success criteria", "Activities", "set induction", "development", "closure", "Teaching aids", "BBM", "Assessment", "Reflection")
	expectInOrder(t, promptOf(t, body), "1. Objective", "2. Success criteria", "3. Activities", "4. Teaching aids", "5. Assessment", "6. Reflection")
	other := regexp.MustCompile(`(?s)<textarea id="rph-prompt-other" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)[1]
	expectAll(t, other, "Objektif", "Kriteria kejayaan", "Aktiviti", "set induksi", "perkembangan", "penutup", "Bahan bantu mengajar (BBM)", "Penilaian", "Refleksi")
	expectInOrder(t, other, "1. Objektif", "2. Kriteria kejayaan", "3. Aktiviti", "4. Bahan bantu mengajar", "5. Penilaian", "6. Refleksi")
	expectAll(t, other, "Standard Kandungan (SK) 1.2: Menjalankan penyiasatan", "Standard Pembelajaran (SP) 1.2.1: Membuat hipotesis")
	expectNone(t, other, "1.1.1")

	// Copying is the browser's own: a button of its own, outside any form, on a page that loads the script that copies.
	expectAll(t, body, `<button class="btn btn-solid" type="button" data-copy="rph-prompt">Copy</button>`, `forms.js`)
	expectNone(t, body, "<form class=\"form\"")
}

// countRPHPrompts is how many RPH prompts every account has made, from the database.
func (s *site) countRPHPrompts() (n int) {
	s.t.Helper()
	if err := s.db.QueryRow(`SELECT count(*) FROM rph_prompts`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// promptOf is the text of the readonly field holding the prompt made.
func promptOf(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<textarea id="rph-prompt" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no prompt in %s", body)
	}
	return html.UnescapeString(m[1])
}
