package app_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	admin, ada, planID := storageSite(t, "1024")
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 GB")
	path := ada.createDashboard("finance", "Kewangan")
	file := bigFinance(t, 1)
	res, body = ada.postFile(path+"/dataset", "mine.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, fmt.Sprintf("%d KB of 1 GB", (len(file)+1023)>>10))

	res, body = admin.post("/admin/plans/"+planID, url.Values{"name": {"Lab Small"}, "days": {"30"}, "deployments": {"3"}, "storage": {"2"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "of 1 GB")
}

func TestAProPeriodGetsTheProPlansStorageAllowance(t *testing.T) {
	s := bclSite(t)
	res, body := s.post("/admin/plans", url.Values{"name": {"Pro Year"}, "days": {"365"}, "deployments": {"5"}, "storage": {"3"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.markProPlan("Pro Year")
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 3 MB")
}

func TestAnUploadOverTheStorageAllowanceIsRefusedAndReplacingOrDeletingFreesSpace(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	first := ada.createDashboard("finance", "One")
	second := ada.createDashboard("finance", "Two")
	file := bigFinance(t, 1)
	res, body := ada.postFile(first+"/dataset", "one.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	used := fmt.Sprintf("%d KB of your 1 MB", (len(file)+1023)>>10)

	res, body = ada.postFile(second+"/dataset", "two.xlsx", bigFinance(t, 2))
	for _, want := range []string{"Storage allowance", used, "Nothing changed."} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files after a refused upload, want 1", n)
	}
	res, body = ada.get(second + "/columns")
	expectStatus(t, res, body, http.StatusOK, "Upload a Dataset first")

	res, body = ada.postFile(first+"/dataset", "again.xlsx", bigFinance(t, 3))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(first+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.postFile(second+"/dataset", "two.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestTwoUploadsAtOnceCannotBothSlipUnderTheAllowance(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	paths := []string{ada.createDashboard("finance", "One"), ada.createDashboard("finance", "Two")}
	files := [][]byte{bigFinance(t, 1), bigFinance(t, 2)}
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := ada.postFile(paths[i]+"/dataset", "data.xlsx", files[i])
			codes[i] = res.StatusCode
		}()
	}
	wg.Wait()
	if !(codes[0] == http.StatusSeeOther) != (codes[1] == http.StatusSeeOther) {
		t.Fatalf("two uploads at once answered %v, want one accepted and one refused", codes)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want 1", n)
	}
}

func TestTheAdminHasNoStorageAllowance(t *testing.T) {
	admin, _, planID := storageSite(t, "0")
	admin.assignPlan("admin@example.com", planID)
	path := admin.createDashboard("finance", "Kewangan")
	res, body := admin.postFile(path+"/dataset", "mine.xlsx", admin.financeSample())
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = admin.get("/lab")
	if strings.Contains(body, "of 0 KB") {
		t.Fatalf("the Admin's Lab shows a Storage allowance: %s", body)
	}
}
