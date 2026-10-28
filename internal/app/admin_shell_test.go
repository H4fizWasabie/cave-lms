package app_test

import (
	"net/http"
	"testing"
)

// Admin pages render in the admin shell: the Admin sidebar without the public site header and footer, with a way
// back to the site, the language switch and Log out in the sidebar. A Learner sent away from /admin keeps the site's.
func TestAdminPagesRenderInTheAdminShellWithoutThePublicHeaderAndFooter(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	for _, path := range []string{"/admin", "/admin/courses", "/admin/courses/new", "/admin/gallery"} {
		for lang, want := range map[string][]string{
			"en": {"View learning site", `action="/language"`, `name="next" value="` + path + `"`, `action="/logout"`, "Log out"},
			"ms": {"Lihat laman pembelajaran", `action="/language"`, `action="/logout"`, "Log keluar"},
		} {
			res, body := s.getAsking(path, lang)
			expectStatus(t, res, body, http.StatusOK, `class="adm-side"`)
			expectNone(t, body, `class="site-header"`, `class="site-footer"`)
			expectAll(t, body, want...)
		}
	}

	// A not-found or error page under /admin has no sidebar, so it keeps the site's header to lead somewhere.
	res, body := s.get("/admin/courses/999999/edit")
	expectStatus(t, res, body, http.StatusNotFound, `class="site-header"`)

	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	_, body = s.get("/admin")
	expectNone(t, body, `class="adm-side"`)
	_, body = s.get("/courses")
	expectAll(t, body, `class="site-header"`, `class="site-footer"`)
}
