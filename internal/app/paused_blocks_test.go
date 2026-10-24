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

	res, body = ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	if res.StatusCode != http.StatusOK || strings.Contains(body, "your header?") {
		t.Fatalf("one column of four missing asks about the header:\n%s", body)
	}
}

func TestRestoringADatasetWithTheColumnUnpausesItsBlocks(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, "is-paused")

	res, body = ada.post(ada.earlier(path)["laporan.xlsx"]+"/restore", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path + "/blocks")
	if res.StatusCode != http.StatusOK || strings.Contains(body, "is-paused") {
		t.Fatalf("blocks still paused after restoring a Dataset with Jumlah:\n%s", body)
	}
}

func TestUseInsteadSwapsTheColumnInEveryBlock(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopFile(t, "Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	before, _ := ada.pageBlocks(path + "/view/")

	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, `<option value="Amaun">Amaun</option>`)
	res, body = ada.post(path+"/blocks", url.Values{"do": {"use"}, "block": {"5"}, "from": {"Jumlah"}, "to": {"Kategori"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "") // a Text column cannot be summed
	ada.editBlocks(path, url.Values{"do": {"use"}, "block": {"5"}, "from": {"Jumlah"}, "to": {"Amaun"}})
	blocks, _ := ada.pageBlocks(path + "/view/")
	if n := using(blocks, "Amaun"); n == 0 || n != using(before, "Jumlah") {
		t.Fatalf("Amaun in %d blocks, Jumlah was in %d", n, using(before, "Jumlah"))
	}
	res, body = ada.get(path + "/blocks")
	if strings.Contains(body, "is-paused") {
		t.Fatalf("blocks still paused after Use instead:\n%s", body)
	}
}

func TestAFilterValueNoLongerInTheDatasetIsFlagged(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	ada.editBlocks(path, barForm(url.Values{"fcol": {"Perkara"}, "fval": {"Beras"}}))
	ada.editBlocks(path, barForm(url.Values{"fcol": {"Perkara"}, "fval": {"Beras"}})) // ticks belong to the column shown
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopReport(t, "Gula", 80))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)

	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, "Beras is not in this Dataset")
	res, body = ada.get(path + "/blocks?open=5")
	expectStatus(t, res, body, http.StatusOK, `name="fval" value="Beras" checked><s>Beras</s>`)
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
	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, `class="eb-item is-paused`)
	expectStatus(t, res, body, http.StatusOK, "Needs <code>Jumlah</code>")

	// A paused block's form still saves, its missing column kept.
	ada.editBlocks(path, barForm(url.Values{"label": {"Belanja"}}))
	blocks, _ := ada.pageBlocks(path + "/view/")
	if blocks[5].Label != "Belanja" || blocks[5].Value.Col != "Jumlah" {
		t.Fatalf("the paused bar after a save: %+v %+v", blocks[5], blocks[5].Value)
	}

	res, body = ada.postFile(path+"/dataset", "disember.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	res, body = ada.get(path + "/blocks")
	if res.StatusCode != http.StatusOK || strings.Contains(body, "is-paused") {
		t.Fatalf("blocks still paused once Jumlah is back:\n%s", body)
	}
}
