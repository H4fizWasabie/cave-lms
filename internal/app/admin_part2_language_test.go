package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestThePlanPagesAndTheirErrorsAreInMalayAndEnglish(t *testing.T) {
	s, planID := adminWithPlanAndLearner(t)
	s.switchLanguage("ms", "/")

	_, body := s.get("/admin/plans")
	expectLanguage(t, body, "ms", "Pentadbir: Pelan")
	expectAll(t, body, "Lab Monthly", "30 hari", "3 Deployment", "Tiada: Pelajar diminta")
	_, body = s.get("/admin/plans/new")
	expectLanguage(t, body, "ms", "Pelan baharu")
	_, body = s.get("/admin/plans/" + planID + "/edit")
	expectLanguage(t, body, "ms", "Sunting Pelan")
	res, body := s.get("/admin/plans/999999/edit")
	expectStatus(t, res, body, http.StatusNotFound, "Pelan tidak dijumpai")
	for _, c := range []struct{ name, days, deployments, link, want string }{
		{" ", "30", "3", "", "Masukkan nama."},
		{"Lab", "0", "3", "", "1 atau lebih"},
		{"Lab", "30", "-1", "", "0 atau lebih"},
		{"Lab", "30", "3", "javascript:alert(1)", "https://"},
	} {
		res, body := s.createPlan(c.name, c.days, c.deployments, c.link)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectLanguage(t, body, "ms", c.want)
	}

	res, body = s.get("/admin/plans/periods?email=nobody@example.com")
	expectStatus(t, res, body, http.StatusNotFound, "Tiada Pelajar dengan e-mel nobody@example.com")
	_, body = s.get("/admin/plans/periods?email=ada@example.com")
	expectLanguage(t, body, "ms", "Tempoh Pelan: Ada")
	expectAll(t, body, "Belum ada Tempoh Pelan.", "Lab Monthly (30 hari, 3 Deployment)", "Tetapkan")
	res, body = s.post("/admin/plans/periods", url.Values{"email": {"ada@example.com"}, "plan_id": {"0"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Pilih Pelan.")
	s.assignPlan("ada@example.com", planID)
	_, body = s.get("/admin/plans/periods?email=ada@example.com")
	expectAll(t, body, "Aktif", "Tamatkan sekarang")
	res, body = s.post("/admin/plans/periods/999999/end", nil)
	expectStatus(t, res, body, http.StatusNotFound, "Tempoh Pelan tidak dijumpai")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/plans")
	expectLanguage(t, body, "en", "Admin: Plans")
	expectAll(t, body, "30 days", "3 Deployments")
	_, body = s.get("/admin/plans/periods?email=ada@example.com")
	expectLanguage(t, body, "en", "Plan periods: Ada")
	expectAll(t, body, "Active", "End now")
}

func TestTheDeploymentsPageAndTakedownErrorsAreInMalayAndEnglish(t *testing.T) {
	admin, ada := labLearner(t)
	ada.deploy(ada.readyDashboard("Kewangan Lama"))
	ada.ageAllDeployments(8)
	ada.deploy(ada.readyDashboard("Kewangan Baru"))
	admin.switchLanguage("ms", "/")

	_, body := admin.get("/admin/deployments")
	expectLanguage(t, body, "ms", "Pentadbir: Deployment")
	expectAll(t, body, "Kewangan Baru", "Dalam talian hingga", "Tidur sejak", "Sebab, ditunjukkan kepada Pelajar", "Turunkan")
	action := takedownAction.FindStringSubmatch(body)[1]
	res, body := admin.post(action, url.Values{"reason": {" "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Berikan sebab Penurunan ini.")
	res, body = admin.post(action, url.Values{"reason": {strings.Repeat("x", 1001)}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "kurang daripada 1,000 aksara")
	admin.post(action, url.Values{"reason": {"Aduan"}})
	_, body = admin.get("/admin/deployments")
	expectAll(t, body, "Diturunkan", "Sebab: Aduan")

	admin.switchLanguage("en", "/")
	_, body = admin.get("/admin/deployments")
	expectLanguage(t, body, "en", "Admin: Deployments")
	expectAll(t, body, "Taken down", "Reason: Aduan", "Asleep since")
}

func TestTheCaveTextLearnersPageIsInMalayAndEnglish(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(42)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.givePro("bea@example.com")
	s.switchLanguage("ms", "/")

	_, body := s.get("/admin/cavetext")
	expectLanguage(t, body, "ms", "Pentadbir: Pelajar CaveText")
	expectAll(t, body, "<strong>43 daripada 1,000</strong> tempat awal telah diambil", "42 (awal)", "Percuma", "Berikan Pro", "Perbaharui Pro", "Tamatkan Pro sekarang")
	res, body := s.post("/admin/cavetext/learners/999999/pro", nil)
	expectStatus(t, res, body, http.StatusNotFound, "Pelajar tidak dijumpai")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/cavetext")
	expectLanguage(t, body, "en", "Admin: CaveText Learners")
	expectAll(t, body, "<strong>43 of 1,000</strong> early places taken", "42 (early)", "Give Pro")
}

func TestTheCaveTextSettingsPageAndItsErrorsAreInMalayAndEnglish(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.createSnippet(token, snippet{Folder: "Work", Name: "Pic", Shortcut: "/pic", Body: "x"})
	s.switchLanguage("ms", "/")

	_, body := s.get("/admin/cavetext/settings")
	expectLanguage(t, body, "ms", "Pentadbir: Tetapan CaveText")
	expectAll(t, body, "Harga Pro (RM setahun)", "Soalan User master prompt", "Master prompt Image", "Simpan master prompt")
	for _, c := range []struct{ price, link, question, want string }{
		{"abc", "", "Q?", "Masukkan harga dalam RM"},
		{"0", "", "Q?", "Masukkan harga lebih daripada 0"},
		{"25", "ftp://x", "Q?", "Masukkan pautan pembayaran"},
		{"25", "", " ", "Masukkan soalan"},
		{"25", "", strings.Repeat("x", 2001), "Pendekkan soalan kepada 2,000 aksara"},
	} {
		res, body := s.post("/admin/cavetext/settings", url.Values{"price": {c.price}, "payment_link": {c.link}, "question": {c.question}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	res, body := s.saveCaveTextSettings("25", "")
	_, body = s.get(res.Header.Get("Location"))
	expectAll(t, body, "Disimpan.")

	for _, c := range []struct {
		form url.Values
		want string
	}{
		{masterPromptForm(" ", "/img", "x", "Dash", "/dash", "y"), "Masukkan nama untuk master prompt Image."},
		{masterPromptForm("Image", "/img", "x", "Dash", "/dash", " "), "Masukkan master prompt Dashboard."},
		{masterPromptForm("Image", "/img", strings.Repeat("x", 20001), "Dash", "/dash", "y"), "Pendekkan master prompt Image kepada 20,000 aksara"},
		{masterPromptForm("Image", "/my img", "x", "Dash", "/dash", "y"), "Master prompt Image: Pintasan tidak boleh mengandungi ruang."},
	} {
		res, body := s.saveMasterPrompts(c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	res, body = s.saveMasterPrompts(masterPromptForm("Picture", "/pic", "Draw it.", "Charts", "/charts", "Chart it."))
	_, body = s.get(res.Header.Get("Location"))
	expectAll(t, body, "Master prompt disimpan.", "1 Pelajar mempunyai Snippet yang pintasannya bertembung")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/cavetext/settings")
	expectLanguage(t, body, "en", "Admin: CaveText settings")
	expectAll(t, body, "1 Learner has a Snippet whose shortcut clashes", "Save master prompts")
}

func TestTheCaveTextCapsPageAndItsErrorIsInMalayAndEnglish(t *testing.T) {
	s := caveTextOwnerSite(t)
	s.switchLanguage("ms", "/")
	_, body := s.get("/admin/cavetext/caps")
	expectLanguage(t, body, "ms", "Pentadbir: Had CaveText")
	expectAll(t, body, "Tempat awal", "Had awal (Snippet)", "Had lewat (Snippet)", "Hanya anda, Pemilik")
	res, body := s.saveCaps("-1", "10", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan nombor bulat dari 0 hingga 1,000,000.")
	res, body = s.saveCaps("1000", "10", "5")
	_, body = s.get(res.Header.Get("Location"))
	expectAll(t, body, "Disimpan.")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/cavetext/caps")
	expectLanguage(t, body, "en", "Admin: CaveText caps")
}

func TestTheSystemLogPageIsInMalayAndEnglishWithEntriesAsLogged(t *testing.T) {
	s := ownerSite(t)
	s.application.Started(":8080")
	s.switchLanguage("ms", "/")
	_, body := s.get("/admin/log")
	expectLanguage(t, body, "ms", "Ralat dan permulaan sistem dalam 30 hari lepas")
	expectAll(t, body, "Ralat sahaja", "Mesej", "Cave LMS started", ">maklumat<")
	_, body = s.get("/admin/log?level=error")
	expectAll(t, body, "Tiada apa-apa dilog: tiada ralat.")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/log")
	expectLanguage(t, body, "en", "Errors and start-ups from the last 30 days")
	expectAll(t, body, "Errors only", "Cave LMS started", ">info<")
}
