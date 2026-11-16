package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// oprAdmin is a site with an Admin, Abah, logged in and writing English.
func oprAdmin(t *testing.T) *site {
	t.Helper()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.switchLanguage("en", "/")
	return s
}

func TestTheAdminSeesFiveOPRStylesStartingAsMarkedPlaceholdersAllShown(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	res, body := s.get("/admin/opr-styles")
	expectStatus(t, res, body, http.StatusOK, "OPR styles")
	expectInOrder(t, body, "Activity (placeholder)", "Achievement (placeholder)", "Co-curricular (placeholder)", "KPI dashboard (placeholder)", "Inspection (placeholder)")
	expectNone(t, body, "Hidden")
	expectAll(t, body, `href="/admin/opr-styles/1/edit"`, `href="/admin/opr-styles/5/edit"`)
}

// saveOPRStyle posts the edit form of the OPR style in slot pos.
func (s *site) saveOPRStyle(pos, name, text string) (*http.Response, string) {
	return s.post("/admin/opr-styles/"+pos, url.Values{"name": {name}, "text": {text}})
}

func TestTheAdminRewritesAnOPRStyleAndTheListAndFormShowIt(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	res, body := s.saveOPRStyle("2", "  Kejohanan  ", "  Use a big trophy layout.  ")
	expectRedirect(t, res, body, "/admin/opr-styles")
	_, body = s.get("/admin/opr-styles")
	expectInOrder(t, body, "Activity (placeholder)", "Kejohanan", "Co-curricular (placeholder)")
	expectNone(t, body, "Achievement (placeholder)")
	_, body = s.get("/admin/opr-styles/2/edit")
	expectAll(t, body, `value="Kejohanan"`, ">Use a big trophy layout.</textarea>")
	expectNone(t, body, "[PLACEHOLDER")
	_, body = s.get("/admin/opr-styles/1/edit")
	expectAll(t, body, "Activity (placeholder)")

	res, body = s.saveOPRStyle("6", "Sixth", "No such slot.")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestAnOPRStyleNeedsANameAndATextWithinTheLimitsAndAnErrorKeepsWhatWasTyped(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	for _, c := range []struct{ name, text, want, kept string }{
		{"   ", "Some text.", "Enter a name.", `value=""`},
		{"Named", "  \n ", "Enter the prompt text.", `value="Named"`},
		{strings.Repeat("n", 81), "Some text.", "The name can be at most 80 characters.", `value="` + strings.Repeat("n", 81) + `"`},
		{"Named", strings.Repeat("t", 4001), "The prompt text can be at most 4,000 characters.", ">" + strings.Repeat("t", 4001) + "</textarea>"},
	} {
		res, body := s.saveOPRStyle("3", c.name, c.text)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, c.kept, `action="/admin/opr-styles/3"`)
	}
	res, body := s.saveOPRStyle("3", strings.Repeat("n", 80), strings.Repeat("t", 4000))
	expectRedirect(t, res, body, "/admin/opr-styles")

	s.saveOPRStyle("4", "", "")
	_, body = s.get("/admin/opr-styles")
	expectAll(t, body, "KPI dashboard (placeholder)")
}

func TestTheAdminHidesAndShowsEachOPRStyle(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	for _, pos := range []string{"1", "2", "3", "4", "5"} {
		res, body := s.post("/admin/opr-styles/"+pos+"/hide", nil)
		expectRedirect(t, res, body, "/admin/opr-styles")
	}
	_, body := s.get("/admin/opr-styles")
	if got := strings.Count(body, "Hidden"); got != 5 {
		t.Fatalf("list shows %d hidden styles, want 5; body:\n%s", got, body)
	}
	expectAll(t, body, "Activity (placeholder)", "Inspection (placeholder)")

	res, body := s.post("/admin/opr-styles/3/show", nil)
	expectRedirect(t, res, body, "/admin/opr-styles")
	_, body = s.get("/admin/opr-styles")
	if got := strings.Count(body, "Hidden"); got != 4 {
		t.Fatalf("list shows %d hidden styles, want 4; body:\n%s", got, body)
	}
	for _, path := range []string{"/admin/opr-styles/6/hide", "/admin/opr-styles/6/show"} {
		res, body = s.post(path, nil)
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
}

func TestOnlyTheAdminReachesTheOPRStylePages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	learner := s.second()
	learner.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	visitor := s.second()
	for _, p := range []struct{ method, path string }{
		{"GET", "/admin/opr-styles"}, {"GET", "/admin/opr-styles/1/edit"}, {"POST", "/admin/opr-styles/1"},
		{"POST", "/admin/opr-styles/1/hide"}, {"POST", "/admin/opr-styles/1/show"},
	} {
		for who, c := range map[string]*site{"Visitor": visitor, "Learner": learner} {
			req := c.newRequest(p.method, p.path, strings.NewReader("name=Hacked&text=Hacked"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, body := c.do(req)
			if who == "Visitor" && (res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login") {
				t.Errorf("%s %s as Visitor = %d to %q, want 303 to /login", p.method, p.path, res.StatusCode, res.Header.Get("Location"))
			}
			if who == "Learner" && res.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s as Learner = %d, want 403; body:\n%s", p.method, p.path, res.StatusCode, body)
			}
		}
	}
	_, body := s.get("/admin/opr-styles")
	expectNone(t, body, "Hacked", "Hidden")
}

func TestTheOPRStylePagesAreInMalayAndEnglish(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	s.saveOPRStyle("1", "Aktiviti", "Susun atur laporan program.")
	s.post("/admin/opr-styles/2/hide", nil)
	s.switchLanguage("ms", "/")
	_, body := s.get("/admin/opr-styles")
	expectLanguage(t, body, "ms", "Gaya OPR")
	expectAll(t, body, "Disembunyikan", "Dipaparkan", "Sunting", "Paparkan", "Sembunyikan", "Aktiviti")
	expectNone(t, body, "OPR styles", "Hidden")
	_, body = s.get("/admin/opr-styles/1/edit")
	expectLanguage(t, body, "ms", "Sunting gaya OPR 1")
	expectAll(t, body, "Teks prompt", "Simpan", "Batal")
	res, body := s.saveOPRStyle("1", "", "x")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan nama.")
	res, body = s.saveOPRStyle("1", "x", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan teks prompt.")
	res, body = s.saveOPRStyle("1", strings.Repeat("n", 81), "x")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Nama boleh sehingga 80 aksara.")
	res, body = s.saveOPRStyle("1", "x", strings.Repeat("t", 4001))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Teks prompt boleh sehingga 4,000 aksara.")
	res, body = s.get("/admin/opr-styles/9/edit")
	expectStatus(t, res, body, http.StatusNotFound, "Gaya OPR tidak dijumpai")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/opr-styles")
	expectLanguage(t, body, "en", "OPR styles")
	expectAll(t, body, "Aktiviti", "Hidden", "Shown")
}

func TestEachOPRStyleOpensForEditingWithItsPlaceholderNameAndTextBasedOnTheCurrentToolsLayouts(t *testing.T) {
	t.Parallel()
	s := oprAdmin(t)
	for pos, want := range map[string][]string{
		"1": {"Activity (placeholder)", "[PLACEHOLDER", "two-column activity information table"},
		"2": {"Achievement (placeholder)", "[PLACEHOLDER", "prominent vertical photo column"},
		"3": {"Co-curricular (placeholder)", "[PLACEHOLDER", "four-photo grid"},
		"4": {"KPI dashboard (placeholder)", "[PLACEHOLDER", "KPI cards"},
		"5": {"Inspection (placeholder)", "[PLACEHOLDER", "corrective action"},
	} {
		res, body := s.get("/admin/opr-styles/" + pos + "/edit")
		expectStatus(t, res, body, http.StatusOK, `name="name"`)
		expectAll(t, body, want...)
	}
	for _, pos := range []string{"0", "6", "one"} {
		res, body := s.get("/admin/opr-styles/" + pos + "/edit")
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
}
