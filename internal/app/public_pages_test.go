package app_test

import (
	"net/http"
	"regexp"
	"testing"
)

var headingTag = regexp.MustCompile(`<h([1-6])[\s>]`)

// expectHeadingsInOrder fails unless a page's content opens with an h1 and never skips a level going down (h1 to h3),
// so a screen reader can move through it by heading.
func expectHeadingsInOrder(t *testing.T, path, body string) {
	t.Helper()
	prev := 0
	for _, m := range headingTag.FindAllStringSubmatch(content(body), -1) {
		level := int(m[1][0] - '0')
		if level > prev+1 {
			t.Errorf("%s: h%d follows h%d", path, level, prev)
		}
		prev = level
	}
	if prev == 0 {
		t.Errorf("%s: no headings", path)
	}
}

func TestThePublicPagesKeepTheirHeadingsInOrderInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("AI Bootcamp", "2099-03-01", "Two days of hands-on AI.", "https://example.com/bootcamp", nil)
	picture := s.uploadAs("Workshop day", "everyone", pngBytes(t))

	visitor := s.second()
	for _, lang := range []string{"en", "ms"} {
		for _, path := range []string{"/about", "/pricing", "/programmes", "/gallery", picture, "/cavetext",
			"/certificates", "/privacy", "/no-such-page"} {
			res, body := visitor.getAsking(path, lang)
			if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
				t.Fatalf("%s: status %d", path, res.StatusCode)
			}
			expectHeadingsInOrder(t, lang+" "+path, body)
		}
	}
}
