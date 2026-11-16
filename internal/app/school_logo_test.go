package app_test

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

// The School logo tick (GLOSSARY "School logo"): on an RPH prompt or an OPR prompt a Teacher ticks "Use my school logo" and the
// prompt says where to place the logo they attach in their chatbot. CAVELMS never takes, stores or shows the picture.

func TestTickingTheSchoolLogoOnAnOPRPromptTellsTheChatbotToPutTheAttachedLogoInTheHeaderBesideTheInstitution(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)

	v := oprForm("1")
	v.Set("logo", "1")
	res, body := teacher.post("/teachers/opr", v)
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	expectAll(t, oprPromptOf(t, body), "SCHOOL LOGO", "Place the attached school logo in the header, beside the institution name", "Do not redraw or alter it")

	_, body = teacher.post("/teachers/opr", oprForm("1")) // not ticked: the prompt is as it was
	expectNone(t, oprPromptOf(t, body), "SCHOOL LOGO", "school logo")
}

func TestTheOPRFormOffersAnUntickedUseMySchoolLogoBoxAndNoFileField(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)
	_, body := teacher.get("/teachers/opr")
	expectAll(t, body, `<input type="checkbox" name="logo" value="1">`, "Use my school logo")
	expectNone(t, body, `type="file"`, `name="logo" value="1" checked`)
	teacher.switchLanguage("ms", "/teachers/opr")
	_, body = teacher.get("/teachers/opr")
	expectAll(t, body, "Guna logo sekolah saya")
}

func TestTheOPRPromptPageRemindsTheTeacherToAttachTheSchoolLogoOnlyWhenTicked(t *testing.T) {
	t.Parallel()
	_, teacher := oprTeacher(t)

	v := oprForm("1")
	v.Set("logo", "1")
	_, body := teacher.post("/teachers/opr", v)
	expectAll(t, body, "Attach your school logo in your chatbot together with this prompt. CAVELMS does not keep it.", "Attach your reference (laporan-hari-sukan.pdf)")
	expectNone(t, body, `type="file"`)

	_, body = teacher.post("/teachers/opr", oprForm("1"))
	expectNone(t, body, "Attach your school logo")

	teacher.switchLanguage("ms", "/teachers/opr")
	_, body = teacher.post("/teachers/opr", v)
	expectAll(t, body, "Lampirkan logo sekolah anda dalam chatbot anda bersama prompt ini. CAVELMS tidak menyimpannya.")
}

func TestTickingTheSchoolLogoOnAnRPHPromptTellsTheChatbotToPutTheAttachedLogoAtTheTopAndReturnADownloadableDocumentInBothLanguages(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")

	v := rphForm(t, form, "1.1.1")
	v.Set("logo", "1")
	res, body := teacher.post("/teachers/dskps/"+id+"/rph", v) // Malay, the DSKP's language
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	expectAll(t, promptOf(t, body), "Letakkan logo sekolah yang dilampirkan di bahagian atas RPH", "dokumen yang boleh dimuat turun (Word atau PDF)")
	other := regexp.MustCompile(`(?s)<textarea id="rph-prompt-other" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)[1]
	expectAll(t, other, "Place the attached school logo at the top of the RPH", "downloadable document (Word or PDF)")

	_, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1")) // not ticked: as it was
	expectNone(t, promptOf(t, body), "logo", "Word atau PDF")
	expectNone(t, regexp.MustCompile(`(?s)<textarea id="rph-prompt-other" readonly[^>]*>(.*?)</textarea>`).FindStringSubmatch(body)[1], "logo", "Word or PDF")
}

func TestTheRPHFormOffersAnUntickedUseMySchoolLogoBoxAndNoFileField(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, body := teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, `<input type="checkbox" name="logo" value="1">`, "Use my school logo")
	expectNone(t, body, `type="file"`, `name="logo" value="1" checked`)
	teacher.switchLanguage("ms", "/")
	_, body = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, "Guna logo sekolah saya")
}

func TestTheRPHPromptPageRemindsTheTeacherToAttachTheSchoolLogoOnlyWhenTicked(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")

	v := rphForm(t, form, "1.1.1")
	v.Set("logo", "1")
	_, body := teacher.post("/teachers/dskps/"+id+"/rph", v)
	expectAll(t, body, "Attach your school logo in your chatbot together with this prompt. CAVELMS does not keep it.")
	expectNone(t, body, `type="file"`)

	_, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectNone(t, body, "Attach your school logo")

	teacher.switchLanguage("ms", "/")
	_, body = teacher.post("/teachers/dskps/"+id+"/rph", v)
	expectAll(t, body, "Lampirkan logo sekolah anda dalam chatbot anda bersama prompt ini. CAVELMS tidak menyimpannya.")
}

func TestTheSchoolLogoChoiceIsKeptOnlyWhenAPromptIsMadeAndPrefillsBothFormsUntilTheNextPromptChangesIt(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	teacher.switchLanguage("en", "/")
	const ticked = `name="logo" value="1" checked`
	rph, opr := "/teachers/dskps/"+id+"/rph", "/teachers/opr"
	rphV := func() url.Values { _, f := teacher.get(rph); return rphForm(t, f, "1.1.1") }
	_, form := teacher.get(rph)
	expectNone(t, form, ticked)

	// An OPR form with a mistake makes no prompt and is not remembered.
	bad := oprForm("1")
	bad.Set("logo", "1")
	bad.Set("title", "")
	res, body := teacher.post(opr, bad)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	_, form = teacher.get(opr)
	expectNone(t, form, ticked)

	// An OPR prompt made with the logo ticked: both forms come back ticked, one setting for both kinds of prompt.
	v := oprForm("1")
	v.Set("logo", "1")
	res, body = teacher.post(opr, v)
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	_, form = teacher.get(opr)
	expectAll(t, form, ticked)
	_, form = teacher.get(rph)
	expectAll(t, form, ticked)

	// An RPH prompt made without it turns it off for both.
	res, body = teacher.post(rph, rphV())
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	_, form = teacher.get(opr)
	expectNone(t, form, ticked)
	_, form = teacher.get(rph)
	expectNone(t, form, ticked)

	// An RPH prompt made with it turns it on again.
	w := rphV()
	w.Set("logo", "1")
	teacher.post(rph, w)
	_, form = teacher.get(opr)
	expectAll(t, form, ticked)

	// A prompt refused for the day's limit changes nothing: this is the third RPH prompt, the next is refused.
	res, body = teacher.post(rph, rphV())
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	_, form = teacher.get(opr)
	expectNone(t, form, ticked)
	over := rphV()
	over.Set("logo", "1")
	res, body = teacher.post(rph, over)
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
	_, form = teacher.get(opr)
	expectNone(t, form, ticked)

	// The Admin has no Teacher profile: a prompt with the logo works and nothing is kept.
	adminV := oprForm("1")
	adminV.Set("logo", "1")
	res, body = admin.post(opr, adminV)
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt")
	_, form = admin.get(opr)
	expectNone(t, form, ticked)
}

func TestTheFormThatSavesAPromptAsASnippetCarriesThePromptOnScreenWithItsSchoolLogoLine(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	v := rphForm(t, form, "1.1.1")
	v.Set("logo", "1")
	_, body := teacher.post("/teachers/dskps/"+id+"/rph", v)
	expectAll(t, html.UnescapeString(hiddenBody.FindStringSubmatch(body)[1]), "Letakkan logo sekolah yang dilampirkan di bahagian atas RPH")

	o := oprForm("1")
	o.Set("logo", "1")
	_, body = teacher.post("/teachers/opr", o)
	expectAll(t, html.UnescapeString(hiddenBody.FindStringSubmatch(body)[1]), "Place the attached school logo in the header")
}

func TestThePrivacyNoticeSaysTheTeacherProfileKeepsWhetherTheSchoolLogoWasTickedAndNeverThePicture(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/privacy", "en")
	expectAll(t, body, "Your Teacher profile keeps whether you last ticked Use my school logo on a prompt, to tick it for you next time. CAVELMS never takes or keeps the logo itself.")
	_, body = s.getAsking("/privacy", "ms")
	expectAll(t, body, "Profil guru anda menyimpan sama ada anda terakhir menandakan Guna logo sekolah saya pada prompt, untuk ditandakan bagi anda pada kali berikutnya. CAVELMS tidak sekali-kali mengambil atau menyimpan logo itu sendiri.")
}
