package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// shopFile is shopReport's table with no title rows, under header, so Clean-up changes nothing.
func shopFile(t *testing.T, header ...string) []byte {
	t.Helper()
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, header,
		[]any{day, "Gula", "Dapur", "Keluar", 80.0},
		[]any{day, "Jualan", "Kedai", "Masuk", 700.0},
	)
}

// using is the blocks that name column.
func using(blocks []lab.WorkBlock, column string) int {
	n := 0
	for _, b := range blocks {
		if b.Value != nil && (b.Value.Col == column || b.Value.A == column) {
			n++
		}
	}
	return n
}

func TestAPickForARenamedColumnSwapsInEveryBlock(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	for _, want := range []string{"Blocks use <code>Jumlah</code>: pick a column or leave it out", `name="use:Jumlah"`,
		`<option value="Amaun">Amaun</option>`, "Leave it out"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, `name="use:Kategori"`) {
		t.Fatal("a column with the same name and type asks for a pick")
	}
	before, _ := ada.pageBlocks(path + "/view/")
	ada.confirmWork(path, url.Values{"use:Jumlah": {"Amaun"}})
	blocks, _ := ada.pageBlocks(path + "/view/")
	if n := using(blocks, "Amaun"); n == 0 || n != using(before, "Jumlah") || using(blocks, "Jumlah") != 0 {
		t.Fatalf("Amaun in %d blocks, Jumlah in %d, was in %d", n, using(blocks, "Jumlah"), using(before, "Jumlah"))
	}
}

func TestAnUnchangedUploadSkipsThePreviewAndGoesLive(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	link := ada.deploy(path)
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		t.Fatalf("an unchanged upload redirects to %q", loc)
	}
	if page := ada.second().rowsOf(link); !strings.Contains(page, "Gula") {
		t.Fatalf("the Deployment's data = %s", page)
	}
}

func TestMostBlockColumnsMissingAsksAboutTheHeaderRow(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Hari", "Barang", "Kumpulan", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Is row 1 your header?")
	expectStatus(t, res, body, http.StatusOK, `name="header" value="1"`) // the answer, ready to change
	if ask, table := strings.Index(body, "your header?"), strings.Index(body, "<table"); ask > table {
		t.Fatal("the preview asks about the header after the rows")
	}
	if strings.Contains(body, `<button class="btn btn-solid">Confirm`) {
		t.Fatal("Confirm is the main action while the header is in question")
	}

	res, body = ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	if res.StatusCode != http.StatusOK || strings.Contains(body, "your header?") {
		t.Fatalf("one column of four missing asks about the header:\n%s", body)
	}
}

func TestReplacingTheDataWithADatasetThatHasTheColumnUnpausesItsBlocks(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if ada.paused(path+"/view/") == 0 {
		t.Fatal("no block paused without Jumlah")
	}

	res, body = ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if n := ada.paused(path + "/view/"); n != 0 {
		t.Fatalf("%d blocks still paused after replacing the data with a Dataset with Jumlah", n)
	}
}

func TestALeftOutColumnPausesItsBlocksUntilItComesBack(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	link := ada.deploy(path)
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, url.Values{"use:Jumlah": {""}})

	// Paused blocks keep their column; the Dashboard and its Deployment leave them off, as the Dataset lacks it.
	for _, page := range []struct {
		who *site
		at  string
	}{{ada, path + "/view/"}, {ada.second(), link}} {
		_, index := page.who.get(page.at)
		if !strings.Contains(index, `"col": "Jumlah"`) || strings.Contains(index, `"Jumlah": {`) {
			t.Fatalf("%s should keep the paused blocks and lack the Jumlah column:\n%s", page.at, index)
		}
	}
	if ada.paused(path+"/view/") == 0 {
		t.Fatal("no block paused without Jumlah")
	}

	res, body = ada.postFile(path+"/dataset", "disember.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if n := ada.paused(path + "/view/"); n != 0 {
		t.Fatalf("%d blocks still paused once Jumlah is back", n)
	}
}

func TestAnUploadWithTheSameKindsOfChangesSkipsThePreview(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady() // its title rows were skipped
	ada.uploadLive(path, shopReport(t, "Gula", 80), "Gula", "Beras")

	res, body := ada.postFile(path+"/dataset", "november.xlsx", dashReport(t, "Garam"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path {
		t.Fatalf("an upload with a new kind of change redirects to %q, not the preview", loc)
	}
}

func TestARetypedColumnLeftOutPausesItsBlocksUntilTheDataIsReplaced(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "november.xlsx", testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, "Gula", "Dapur", "Keluar", "lapan puluh"}, []any{day, "Jualan", "Kedai", "Masuk", "tujuh ratus"}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, url.Values{"use:Jumlah": {""}})
	if ada.paused(path+"/view/") == 0 {
		t.Fatal("no block paused with Jumlah retyped")
	}

	res, body = ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if n := ada.paused(path + "/view/"); n != 0 {
		t.Fatalf("%d blocks still paused after replacing the data with a Dataset with a Number Jumlah", n)
	}
}
