package app_test

import (
	"html"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// rphCaveTextTeacher is rphTeacher on a site that also runs CaveText: the Teacher, Ada, has not signed in to CaveText yet.
func rphCaveTextTeacher(t *testing.T) (admin, teacher *site, id string) {
	t.Helper()
	admin = newSite(t, func(c *config.Config) { c.CaveTextExtensionIDs = []string{extensionID} })
	admin.loginAsNewAdmin("Abah", "admin@example.com", adminPassword)
	id = path.Base(admin.publishedDSKP("Sains", "Y4"))
	return admin, newDSKPTeacher(admin), id
}

var (
	saveForm   = regexp.MustCompile(`(?s)<form[^>]*action="/teachers/rph/snippet"[^>]*>(.*?)</form>`)
	hiddenBody = regexp.MustCompile(`<input type="hidden" name="body" value="([^"]*)">`)
	nameValue  = regexp.MustCompile(`name="name" value="([^"]*)"`)
)

// saveSnippet posts the form the prompt page offers for saving the prompt as a Snippet, with a shortcut typed.
func (s *site) saveSnippet(t *testing.T, promptPage, shortcut string) (*http.Response, string) {
	t.Helper()
	f := saveForm.FindStringSubmatch(promptPage)
	if f == nil {
		t.Fatalf("no form to save the prompt as a Snippet in %s", promptPage)
	}
	body, name := hiddenBody.FindStringSubmatch(f[1]), nameValue.FindStringSubmatch(f[1])
	if body == nil || name == nil {
		t.Fatalf("the save form has no body or name: %s", f[1])
	}
	return s.post("/teachers/rph/snippet", url.Values{"body": {html.UnescapeString(body[1])}, "name": {html.UnescapeString(name[1])}, "shortcut": {shortcut}})
}

func TestATeacherSavesTheRPHPromptAsASnippetAndTheCaveTextAccountIsMadeIfMissing(t *testing.T) {
	t.Parallel()
	_, teacher, id := rphCaveTextTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	_, page := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	prompt := promptOf(t, page)

	res, body := teacher.saveSnippet(t, page, ";rphsains")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")

	// Ada never signed in to CaveText before; her first sign-in finds the Snippet in the account made for it.
	got := teacher.snippets(teacher.connect())
	if len(got) != 1 || got[0].Body != prompt || got[0].Shortcut != ";rphsains" || got[0].Folder != "RPH" || got[0].Name != "RPH Sains SK 1.1" {
		t.Fatalf("Snippets = %s, want the prompt as RPH/RPH Sains SK 1.1 with shortcut ;rphsains", describe(got))
	}
}

func TestTheLastKelasMasaAndNumberOfMuridTypedArePrefilledNextTime(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphTeacher(t)
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	expectNone(t, form, `value="8.00 - 9.00 pagi"`, `value="4 Bestari"`, `value="31"`)

	// A form with a mistake makes no prompt and is not remembered.
	bad := rphForm(t, form, "1.1.1")
	bad.Set("students", "0")
	bad.Set("class", "5 Cerdik")
	res, body := teacher.post("/teachers/dskps/"+id+"/rph", bad)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	_, form = teacher.get("/teachers/dskps/" + id + "/rph")
	expectNone(t, form, "5 Cerdik")

	res, body = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectStatus(t, res, body, http.StatusOK, "Your RPH prompt")
	_, form = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, form, `name="time" value="8.00 - 9.00 pagi"`, `name="class" value="4 Bestari"`, `name="students" value="31"`)
	expectNone(t, form, "PAK21, BBM: kad gambar") // the notes are for the lesson, not kept

	// The latest made wins, and it is the Teacher's own: another's form is still empty.
	later := rphForm(t, form, "1.1.2")
	later.Set("time", "60 minit")
	later.Set("class", "4 Cerdik")
	later.Set("students", "28")
	teacher.post("/teachers/dskps/"+id+"/rph", later)
	_, form = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, form, `name="time" value="60 minit"`, `name="class" value="4 Cerdik"`, `name="students" value="28"`)
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	bob.saveTeacherProfile("Sains", "Y4")
	_, form = bob.get("/teachers/dskps/" + id + "/rph")
	expectNone(t, form, "60 minit", "4 Cerdik", `value="28"`)
}

func TestSavingAnRPHPromptAsASnippetIsNotAPromptMadeAndWorksAtTheDailyLimit(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphCaveTextTeacher(t)
	asked := len(admin.ai.DSKPRequests())
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	_, page := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	_, page = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))

	// Saving the prompt on screen uses none of the day's prompts.
	res, body := teacher.saveSnippet(t, page, ";rph1")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")
	if n := teacher.countRPHPrompts(); n != 2 {
		t.Errorf("%d prompts are counted after saving, want the 2 made", n)
	}
	_, body = teacher.get("/teachers/dskps/" + id + "/rph")
	expectAll(t, body, "1 of 3 RPH prompts left today")

	// With the day's 3 made, it can still be saved.
	_, page = teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	expectAll(t, page, "0 of 3 RPH prompts left today")
	res, body = teacher.saveSnippet(t, page, ";rph2")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")
	if n := teacher.countRPHPrompts(); n != 3 {
		t.Errorf("%d prompts are counted after saving at the limit, want the 3 made", n)
	}
	if got := len(admin.ai.DSKPRequests()); got != asked {
		t.Errorf("saving asked the AI: %d requests, was %d", got, asked)
	}
	if got := teacher.snippets(teacher.connect()); len(got) != 2 {
		t.Errorf("Snippets = %s, want the 2 saved", describe(got))
	}
}

func TestAnRPHPromptIsSavedUnderTheSnippetRulesTheCapTheShortcutRulesAndOnlyForATeacher(t *testing.T) {
	t.Parallel()
	admin, teacher, id := rphCaveTextTeacher(t)
	token := teacher.connect()
	_, form := teacher.get("/teachers/dskps/" + id + "/rph")
	_, page := teacher.post("/teachers/dskps/"+id+"/rph", rphForm(t, form, "1.1.1"))
	teacher.createSnippet(token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})

	// Shortcuts: one is needed, with no spaces, and not clashing with another's, as in CaveText itself.
	for _, c := range []struct{ shortcut, want string }{
		{"", "A shortcut is needed."},
		{";rph one", "no spaces"},
		{"/review", "already uses /review."},
		{"/rev", "/review"},
	} {
		res, body := teacher.saveSnippet(t, page, c.shortcut)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, `action="/teachers/rph/snippet"`) // the form comes back to try again
	}

	// The cap: a Teacher who has reached it is refused, with CaveText's own message, and nothing more is saved.
	me := teacher.me(token)
	teacher.fillSnippets(token, me.Cap-1)
	res, body := teacher.saveSnippet(t, page, ";rphcap")
	expectStatus(t, res, body, http.StatusForbidden, "")
	if got := teacher.snippets(token); len(got) != me.Cap {
		t.Errorf("%d Snippets after the refused save, want the %d the cap allows", len(got), me.Cap)
	}

	// Someone not logged in is sent to log in, not to CaveText.
	res, body = admin.second().post("/teachers/rph/snippet", url.Values{"body": {"x"}, "name": {"x"}, "shortcut": {"x"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
}
