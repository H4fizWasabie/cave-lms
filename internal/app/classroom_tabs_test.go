package app_test

import (
	"fmt"
	"strings"
	"testing"
)

// classroomTabsOf is the Classroom tabs nav in a page's main part, or "" when it has none.
func classroomTabsOf(body string) string {
	_, rest, ok := strings.Cut(mainOf(body), `<nav class="lab-tabs classroom-tabs" aria-label="`)
	if !ok {
		return ""
	}
	nav, _, _ := strings.Cut(rest, "</nav>")
	return nav
}

// classroomPages are the four Classroom pages, as a Learner opens them, with the label of their tab in each language.
var classroomPages = []struct {
	path, en, ms string
}{
	{"/my-courses", "My courses", "Kursus saya"},
	{"/my-exams", "My exams", "Peperiksaan saya"},
	{"/my-folder", "My folder", "Folder saya"},
	{"/my-profile", "My profile", "Profil saya"},
}

func TestEveryClassroomPageOpensWithTheFourTabsAndMarksItsOwnAsTheCurrentPage(t *testing.T) {
	t.Parallel()
	for _, lang := range []string{"en", "ms"} {
		s := newSite(t)
		s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
		for i, page := range classroomPages {
			var body string
			if page.path == "/my-folder" {
				_, body = s.getFolderAsking(lang)
			} else {
				_, body = s.getAsking(page.path, lang)
			}
			nav := classroomTabsOf(body)
			if nav == "" {
				t.Fatalf("%s in %s has no Classroom tabs: %s", page.path, lang, mainOf(body))
			}
			var wants []string
			for j, tab := range classroomPages {
				label := tab.en
				if lang == "ms" {
					label = tab.ms
				}
				href := tab.path
				if href == "/my-folder" { // the tab links straight to the Learner's own Folder, no redirect on the way
					href += "/" + usernameFor("ada@example.com")
				}
				current := ""
				if j == i {
					current = ` aria-current="page"`
				}
				wants = append(wants, fmt.Sprintf(`<a class="lab-tab" href="%s"%s>%s</a>`, href, current, label))
			}
			expectInOrder(t, nav, wants...)
			if n := strings.Count(nav, `aria-current="page"`); n != 1 {
				t.Errorf("%s in %s marks %d tabs as the current page, want 1: %s", page.path, lang, n, nav)
			}
			if strings.Contains(nav, `role="tab`) {
				t.Errorf("%s in %s uses an ARIA tablist, want plain links: %s", page.path, lang, nav)
			}
		}
	}
}

func TestTheAdminReadingALearnersFolderSeesNoClassroomTabs(t *testing.T) {
	t.Parallel()
	admin, _, _, _ := adaWithWorkAndPlan(t)
	res, body := admin.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, 200, "<h1>Folder of Ada</h1>")
	if nav := classroomTabsOf(body); nav != "" {
		t.Errorf("the Admin's view of a Folder has Classroom tabs: %s", nav)
	}
}
