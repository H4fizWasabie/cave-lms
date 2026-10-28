package app_test

import (
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// storageSite is the Admin, Ada and a 30-day Plan of 3 Deployments and storage MB, assigned to Ada.
func storageSite(t *testing.T, storage string) (admin, ada *site, planID string) {
	t.Helper()
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := admin.post("/admin/plans", url.Values{"name": {"Lab Small"}, "days": {"30"}, "deployments": {"3"}, "storage": {storage}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	planID = admin.onlyPlanID()
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	admin.assignPlan("ada@example.com", planID)
	return admin, ada, planID
}

// bigFinance is a valid Finance Dataset of about 600 KB: text that does not compress keeps the .xlsx large.
func bigFinance(t *testing.T, seed int64) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(seed))
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	var rows [][]any
	for i := 0; i < 3500; i++ {
		b := make([]byte, 200)
		for j := range b {
			b[j] = letters[rnd.Intn(len(letters))]
		}
		rows = append(rows, []any{day, string(b), "Dapur", "Keluar", float64(i)})
	}
	file := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, rows...)
	if n := len(file); n < 550<<10 || n > 900<<10 {
		t.Fatalf("bigFinance is %d bytes, want 550 to 900 KB", n)
	}
	return file
}

func TestTheAdminSetsAStorageAllowanceOnEachPlan(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.get("/admin/plans/new")
	expectStatus(t, res, body, http.StatusOK, `name="storage" value="1024"`)
	res, body = s.post("/admin/plans", url.Values{"name": {"Pro Year"}, "days": {"365"}, "deployments": {"5"}, "storage": {"1024"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/plans")
	expectStatus(t, res, body, http.StatusOK, "1 GB")
	id := s.onlyPlanID()
	res, body = s.post("/admin/plans/"+id, url.Values{"name": {"Pro Year"}, "days": {"365"}, "deployments": {"5"}, "storage": {"500"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/plans/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `name="storage" value="500"`)
	for _, bad := range []string{"", "-1", "lots"} {
		res, body = s.post("/admin/plans/"+id, url.Values{"name": {"Pro Year"}, "days": {"365"}, "deployments": {"5"}, "storage": {bad}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the Storage allowance in MB")
	}
}

func TestTheLabShowsStorageUsedAndPeriodsKeepTheirAllowance(t *testing.T) {
	t.Parallel()
	admin, ada, planID := storageSite(t, "1024")
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 GB")
	path := ada.createWork("Kewangan")
	ada.confirmFile(path, "mine.xlsx", bigFinance(t, 1))
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, ada.storedKB())

	res, body = admin.post("/admin/plans/"+planID, url.Values{"name": {"Lab Small"}, "days": {"30"}, "deployments": {"3"}, "storage": {"2"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "of 1 GB")
}

func TestALabPlanPeriodGetsTheLabPlansStorageAllowance(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	res, body := s.post("/admin/plans", url.Values{"name": {"Lab Year"}, "days": {"365"}, "deployments": {"5"}, "storage": {"3"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.markLabPlan("Lab Year")
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidLabPlan("LINK-1", "ada@example.com"))
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 3 MB")
}

func TestAnUploadOverTheStorageAllowanceIsRefusedAndDeletingFreesSpace(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "2")
	first := ada.createWork("One")
	second := ada.createWork("Two")
	ada.confirmFile(first, "one.xlsx", bigFinance(t, 1))

	res, body := ada.postFile(second+"/dataset", "two.xlsx", bigFinance(t, 2))
	// The refused Dataset is dropped; its Workbook stays and counts.
	used := strings.TrimSuffix(ada.storedKB(), " of 1 GB") + " of your 2 MB"
	for _, want := range []string{"Storage allowance", used} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	res, body = ada.get(second + "/view/")
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = ada.post(first+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	// Deleting a Dashboard frees its Dataset, not the Workbooks kept from its uploads; deleting those is a later step.
	ada.forgetWorkbooks()
	ada.confirmFile(second, "two.xlsx", bigFinance(t, 2))
}
