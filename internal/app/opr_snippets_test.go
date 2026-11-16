package app_test

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// oprCaveTextTeacher is oprTeacher on a site that also runs CaveText: the Teacher, Ada, has not signed in to CaveText yet.
func oprCaveTextTeacher(t *testing.T) (admin, teacher *site) {
	t.Helper()
	admin = newSite(t, func(c *config.Config) { c.CaveTextExtensionIDs = []string{extensionID} })
	admin.loginAsNewAdmin("Abah", "admin@example.com", adminPassword)
	admin.switchLanguage("en", "/")
	teacher = newDSKPTeacher(admin)
	teacher.switchLanguage("en", "/")
	return admin, teacher
}

var oprSaveForm = regexp.MustCompile(`(?s)<form[^>]*action="/teachers/opr/snippet"[^>]*>(.*?)</form>`)

// saveOPRSnippet posts the form the OPR prompt page offers for saving the prompt as a Snippet, with a shortcut typed.
func (s *site) saveOPRSnippet(t *testing.T, promptPage, shortcut string) (*http.Response, string) {
	t.Helper()
	f := oprSaveForm.FindStringSubmatch(promptPage)
	if f == nil {
		t.Fatalf("no form to save the OPR prompt as a Snippet in %s", promptPage)
	}
	body, name := hiddenBody.FindStringSubmatch(f[1]), nameValue.FindStringSubmatch(f[1])
	if body == nil || name == nil {
		t.Fatalf("the save form has no body or name: %s", f[1])
	}
	return s.post("/teachers/opr/snippet", url.Values{"body": {html.UnescapeString(body[1])}, "name": {html.UnescapeString(name[1])}, "shortcut": {shortcut}})
}

func TestATeacherSavesTheOPRPromptAsASnippetInFolderOPRNamedFromTheTajukAndTheCaveTextAccountIsMadeIfMissing(t *testing.T) {
	t.Parallel()
	_, teacher := oprCaveTextTeacher(t)
	_, page := teacher.post("/teachers/opr", oprForm("1"))
	prompt := oprPromptOf(t, page)

	res, body := teacher.saveOPRSnippet(t, page, ";oprsukan")
	expectStatus(t, res, body, http.StatusOK, "Your OPR prompt is saved as a Snippet in CaveText. Type ;oprsukan to insert it.")

	// Ada never signed in to CaveText before; her first sign-in finds the Snippet in the account made for it.
	got := teacher.snippets(teacher.connect())
	if len(got) != 1 || got[0].Body != prompt || got[0].Shortcut != ";oprsukan" || got[0].Folder != "OPR" || got[0].Name != "Hari Sukan Tahunan 2027" {
		t.Fatalf("Snippets = %s, want the prompt as OPR/Hari Sukan Tahunan 2027 with shortcut ;oprsukan", describe(got))
	}

	// A tajuk longer than a Snippet's name can be is cut to fit, so saving still works.
	v := oprForm("1")
	v.Set("title", strings.Repeat("x", 150))
	_, page = teacher.post("/teachers/opr", v)
	res, body = teacher.saveOPRSnippet(t, page, ";oprlong")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")
	if got := teacher.snippets(teacher.connect()); len(got) != 2 || got[1].Name != strings.Repeat("x", 100) {
		t.Errorf("Snippets = %s, want the second named by the first 100 characters of the tajuk", describe(got))
	}
}

func TestSavingAnOPRPromptAsASnippetIsNotAPromptMadeAndWorksAtTheDailyLimit(t *testing.T) {
	t.Parallel()
	_, teacher := oprCaveTextTeacher(t)
	_, page := teacher.post("/teachers/opr", oprForm("1"))
	_, page = teacher.post("/teachers/opr", oprForm("1"))

	// Saving the prompt on screen uses none of the day's prompts.
	res, body := teacher.saveOPRSnippet(t, page, ";opr1")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")
	if n := teacher.countOPRPrompts(); n != 2 {
		t.Errorf("%d OPR prompts are counted after saving, want the 2 made", n)
	}
	_, body = teacher.get("/teachers/opr")
	expectAll(t, body, "1 of 3 OPR prompts left today")

	// With the day's 3 made, it can still be saved.
	_, page = teacher.post("/teachers/opr", oprForm("1"))
	expectAll(t, page, "0 of 3 OPR prompts left today")
	res, body = teacher.saveOPRSnippet(t, page, ";opr2")
	expectStatus(t, res, body, http.StatusOK, "saved as a Snippet")
	if n := teacher.countOPRPrompts(); n != 3 {
		t.Errorf("%d OPR prompts are counted after saving at the limit, want the 3 made", n)
	}
	if got := teacher.snippets(teacher.connect()); len(got) != 2 {
		t.Errorf("Snippets = %s, want the 2 saved", describe(got))
	}
}

func TestAnOPRPromptIsSavedUnderTheSnippetRulesTheCapTheShortcutRulesAndOnlyForATeacher(t *testing.T) {
	t.Parallel()
	admin, teacher := oprCaveTextTeacher(t)
	token := teacher.connect()
	_, page := teacher.post("/teachers/opr", oprForm("1"))
	teacher.createSnippet(token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})

	// Shortcuts: one is needed, with no spaces, and not clashing with another's, as in CaveText itself.
	for _, c := range []struct{ shortcut, want string }{
		{"", "A shortcut is needed."},
		{";opr one", "no spaces"},
		{"/review", "already uses /review."},
		{"/rev", "/review"},
	} {
		res, body := teacher.saveOPRSnippet(t, page, c.shortcut)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, `action="/teachers/opr/snippet"`) // the form comes back to try again
	}

	// The cap: a Teacher who has reached it is refused, with CaveText's own message, and nothing more is saved.
	me := teacher.me(token)
	teacher.fillSnippets(token, me.Cap-1)
	res, body := teacher.saveOPRSnippet(t, page, ";oprcap")
	expectStatus(t, res, body, http.StatusForbidden, "")
	if got := teacher.snippets(token); len(got) != me.Cap {
		t.Errorf("%d Snippets after the refused save, want the %d the cap allows", len(got), me.Cap)
	}

	// Someone not logged in is sent to log in, not to CaveText.
	res, body = admin.second().post("/teachers/opr/snippet", url.Values{"body": {"x"}, "name": {"x"}, "shortcut": {"x"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
}
