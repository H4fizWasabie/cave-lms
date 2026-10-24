package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// workReady is a Work Dashboard on Finance with shopReport confirmed, its Perkara column left out of the blanks.
func (s *site) workReady() string {
	s.t.Helper()
	path := s.createWork("finance", "Kedai")
	res, body := s.postFile(path+"/dataset", "laporan.xlsx", shopReport(s.t, "Beras", 120))
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	s.confirmWork(path, url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Kategori"}, "type": {"Jenis"}})
	return path
}

var pageConfig = regexp.MustCompile(`(?s)const dashboard = (.*?);\n`)

// pageBlocks is the blocks and filter bar the Dashboard page at page (ending in /) draws.
func (s *site) pageBlocks(page string) ([]lab.WorkBlock, []string) {
	s.t.Helper()
	res, body := s.get(page)
	expectStatus(s.t, res, body, http.StatusOK, "")
	m := pageConfig.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("%s has no config: %s", page, body)
	}
	var cfg struct {
		Blocks  []lab.WorkBlock `json:"blocks"`
		Filters []string        `json:"filters"`
	}
	if err := json.Unmarshal([]byte(m[1]), &cfg); err != nil {
		s.t.Fatalf("%s config: %v\n%s", page, err, m[1])
	}
	return cfg.Blocks, cfg.Filters
}

// editBlocks posts one edit to the Blocks tab and expects it saved.
func (s *site) editBlocks(path string, form url.Values) {
	s.t.Helper()
	res, body := s.post(path+"/blocks", form)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, path+"/blocks?") || !strings.Contains(loc, "saved") {
		s.t.Fatalf("an edit redirects to %q", loc)
	}
}

func labels(blocks []lab.WorkBlock) string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.Label)
	}
	return strings.Join(out, ", ")
}

func TestTheBlocksTabListsTheBlocksBesideTheDashboard(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	res, body := ada.get(path + "/blocks")
	for _, want := range []string{
		`href="` + path + `/blocks" aria-current="page">2. Blocks`, "Edit blocks",
		"Money in", "Spending by category", "Sum of Jumlah · Kategori · Jenis = Keluar", "Latest transactions",
		"8/20 · Cards 4/8", `src="` + path + `/view/"`, "Filter bar",
	} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/blocks"`)
	for i, want := range []string{`name="kind" value="card" checked`, `name="agg"`, `name="agg"`, `name="agg"`, `name="x"`, `name="fval" value="Keluar" checked`,
		`name="group"`, `name="cols" value="Tarikh" checked`} {
		res, body = ada.get(fmt.Sprintf("%s/blocks?open=%d", path, i))
		expectStatus(t, res, body, http.StatusOK, want)
		expectStatus(t, res, body, http.StatusOK, `name="close" value="1"`) // Done
	}

	practice := ada.readyDashboard("Latihan")
	res, body = ada.get(practice + "/blocks")
	expectStatus(t, res, body, http.StatusNotFound, "")

	ada.switchLanguage("ms", path+"/blocks")
	res, body = ada.get(path + "/blocks")
	for _, want := range []string{"2. Blok", "Ubah blok", "Kad 4/8", "Bar penapis"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

func TestAnAddedBlockShowsOnTheDeploymentAtOnce(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	link := ada.deploy(path)
	viewer := ada.second()

	ada.editBlocks(path, url.Values{"do": {"add"}, "kind": {"bar"}})
	res, body := ada.get(path + "/blocks?saved&open=8")
	expectStatus(t, res, body, http.StatusOK, "Saved. The live link shows it now.")
	expectStatus(t, res, body, http.StatusOK, `name="group"`) // its form is open
	for _, page := range []*site{ada, viewer} {
		at := link
		if page == ada {
			at = path + "/view/"
		}
		blocks, _ := page.pageBlocks(at)
		if len(blocks) != 9 || blocks[8].Kind != "bar" || blocks[8].Group == "" || blocks[8].Top != 8 {
			t.Fatalf("%s blocks after adding a bar: %+v", at, blocks)
		}
	}

	ada.editBlocks(path, url.Values{"do": {"add"}, "kind": {"card"}})
	blocks, _ := ada.pageBlocks(path + "/view/")
	if blocks[4].Kind != "card" || blocks[4].Label != "Rows" {
		t.Fatalf("a new card goes after the cards, labelled Rows: %s", labels(blocks))
	}
}

// barForm is the open form of the Spending by category bar (block 5) with fields set over its saved choices.
func barForm(set url.Values) url.Values {
	f := url.Values{"do": {"save"}, "block": {"5"}, "kind": {"bar"}, "label": {"Spending by category"}, "agg": {"sum"}, "col": {"Jumlah"},
		"group": {"Kategori"}, "top": {"8"}, "fcol": {"Jenis"}, "fval": {"Keluar"}}
	for k, v := range set {
		f[k] = v
	}
	return f
}

func TestAChangedBlockUsesAnyColumnAndItsLabelFollowsUntilTyped(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()

	// Perkara is no blank of the Template the Dashboard opened with.
	ada.editBlocks(path, barForm(url.Values{"group": {"Perkara"}, "agg": {"avg"}, "split": {"Kategori"}, "top": {"0"}}))
	blocks, _ := ada.pageBlocks(path + "/view/")
	b := blocks[5]
	if b.Group != "Perkara" || b.Value.Agg != "avg" || b.Split != "Kategori" || b.Top != 0 || b.Label != "Average Jumlah by Perkara" || b.LabelSet {
		t.Fatalf("the changed bar is %+v %+v", b, b.Value)
	}
	if b.Filter == nil || b.Filter.Col != "Jenis" || len(b.Filter.Vals) != 1 {
		t.Fatalf("the bar's filter is %+v", b.Filter)
	}

	ada.editBlocks(path, barForm(url.Values{"label": {"Purata belanja"}, "group": {"Perkara"}, "agg": {"avg"}}))
	ada.editBlocks(path, barForm(url.Values{"label": {"Purata belanja"}, "group": {"Kategori"}, "agg": {"avg"}}))
	blocks, _ = ada.pageBlocks(path + "/view/")
	if blocks[5].Label != "Purata belanja" || !blocks[5].LabelSet {
		t.Fatalf("a typed label changed with the choices: %+v", blocks[5])
	}
	ada.editBlocks(path, barForm(url.Values{"label": {""}, "group": {"Kategori"}, "agg": {"distinct"}, "col": {"Perkara"}}))
	blocks, _ = ada.pageBlocks(path + "/view/")
	if blocks[5].Label != "Distinct Perkara by Kategori" || blocks[5].LabelSet {
		t.Fatalf("an emptied label does not follow the choices again: %+v", blocks[5])
	}

	// A width change keeps a Template label; a kind change keeps the label, value, filter and width.
	donut := url.Values{"do": {"save"}, "block": {"6"}, "kind": {"donut"}, "label": {"Money in by source"}, "agg": {"sum"},
		"col": {"Jumlah"}, "group": {"Kategori"}, "fcol": {"Jenis"}, "fval": {"Masuk"}, "width": {"full"}}
	ada.editBlocks(path, donut)
	donut.Set("kind", "bar")
	ada.editBlocks(path, donut)
	blocks, _ = ada.pageBlocks(path + "/view/")
	if b := blocks[6]; b.Kind != "bar" || b.Label != "Money in by source" || !b.Wide || b.Top != 8 || b.Filter.Vals[0] != "Masuk" {
		t.Fatalf("the donut turned bar is %+v", b)
	}

	// A table shows the ticked columns, sorted, 25 rows.
	ada.editBlocks(path, url.Values{"do": {"save"}, "block": {"7"}, "kind": {"table"}, "label": {"Latest transactions"},
		"cols": {"Tarikh", "Perkara"}, "sort": {"Jumlah"}, "limit": {"25"}, "width": {"half"}, "close": {"1"}})
	blocks, _ = ada.pageBlocks(path + "/view/")
	if b := blocks[7]; strings.Join(b.Cols, ",") != "Tarikh,Perkara" || b.Sort != "Jumlah" || b.Limit != 25 || b.Wide {
		t.Fatalf("the table is %+v", b)
	}
	// A calculation, row by row, then averaged.
	ada.editBlocks(path, url.Values{"do": {"save"}, "block": {"1"}, "kind": {"card"}, "label": {"Money in"}, "agg": {"calc"},
		"a": {"Jumlah"}, "op": {"x"}, "b": {"Jumlah"}, "then": {"avg"}})
	blocks, _ = ada.pageBlocks(path + "/view/")
	if v := blocks[1].Value; v.Agg != "calc" || v.Op != "x" || v.Then != "avg" || blocks[1].Label != "Jumlah × Jumlah" {
		t.Fatalf("the calculation card is %+v %+v", blocks[1], v)
	}
}

func TestBlocksMoveAmongTheirOwnSortAndGo(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	ada.editBlocks(path, url.Values{"do": {"down"}, "block": {"3"}}) // the last card stays among the cards
	ada.editBlocks(path, url.Values{"do": {"up"}, "block": {"4"}})   // the first chart too
	ada.editBlocks(path, url.Values{"do": {"up"}, "block": {"3"}})
	ada.editBlocks(path, url.Values{"do": {"down"}, "block": {"5"}})
	ada.editBlocks(path, url.Values{"do": {"remove"}, "block": {"0"}})
	blocks, _ := ada.pageBlocks(path + "/view/")
	if got, want := labels(blocks), "Money in, Transactions, Money out, By month, Money in by source, Spending by category, Latest transactions"; got != want {
		t.Fatalf("blocks after moving and removing: %s, want %s", got, want)
	}
}

func TestTheServerKeepsBlocksWithinLimitsAndTheDataset(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	for _, f := range []url.Values{
		barForm(url.Values{"group": {"Pelanggan"}}),
		barForm(url.Values{"top": {"7"}}),
		barForm(url.Values{"kind": {"pie"}}),
		barForm(url.Values{"fcol": {"Jumlah"}}),
		barForm(url.Values{"label": {strings.Repeat("a", 81)}}),
		{"do": {"filter-bar"}, "bar": {"Jumlah"}},
	} {
		res, body := ada.post(path+"/blocks", f)
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `class="up-err" role="alert"`) {
			t.Fatalf("%v = %d, want refused:\n%s", f, res.StatusCode, body)
		}
	}
	res, body := ada.post(path+"/blocks", barForm(url.Values{"group": {"Pelanggan"}}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This Dataset has no column called Pelanggan.")
	blocks, _ := ada.pageBlocks(path + "/view/")
	if blocks[5].Group != "Kategori" {
		t.Fatalf("a refused edit was saved: %+v", blocks[5])
	}

	for range 4 {
		ada.editBlocks(path, url.Values{"do": {"add"}, "kind": {"card"}})
	}
	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, "12/20 · Cards 8/8")
	expectStatus(t, res, body, http.StatusOK, `name="kind" value="card" disabled`)
	res, body = ada.post(path+"/blocks", url.Values{"do": {"add"}, "kind": {"card"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "at most 8 cards")
	res, body = ada.post(path+"/blocks", url.Values{"do": {"save"}, "block": {"8"}, "kind": {"card"}, "agg": {"count"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "at most 8 cards")
	for range 8 {
		ada.editBlocks(path, url.Values{"do": {"add"}, "kind": {"table"}})
	}
	res, body = ada.post(path+"/blocks", url.Values{"do": {"add"}, "kind": {"line"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "at most 20 blocks")
	if blocks, _ := ada.pageBlocks(path + "/view/"); len(blocks) != 20 {
		t.Fatalf("%d blocks", len(blocks))
	}
}

func TestTheFilterBarIsTheTickedColumns(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	link := ada.deploy(path)
	ada.editBlocks(path, url.Values{"do": {"filter-bar"}, "bar": {"Perkara", "Tarikh"}})
	_, filters := ada.second().pageBlocks(link)
	if strings.Join(filters, ",") != "Perkara,Tarikh" {
		t.Fatalf("the Deployment's filter bar is %v", filters)
	}
	res, body := ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, `name="bar" value="Perkara" checked`)
}

func TestTheTemplatePickerShowsEachDashboardQuestion(t *testing.T) {
	_, ada := labLearner(t)
	for _, page := range []string{"/lab/new", "/lab/new?kind=work"} {
		res, body := ada.get(page)
		expectStatus(t, res, body, http.StatusOK, "Am I spending more than I earn?")
	}
	res, body := ada.second().get("/lab") // a visitor sees the Templates
	expectStatus(t, res, body, http.StatusOK, "Am I spending more than I earn?")
}

func TestTheFilterBarTakesOneDateColumn(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "tugas.xlsx", testutil.XLSX(t, []string{"Mula", "Tamat", "Nota"}, []any{day, day, "Cat"}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", url.Values{"template": {"general"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/blocks", url.Values{"do": {"filter-bar"}, "bar": {"Mula", "Tamat"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "one Date column")
	ada.editBlocks(path, url.Values{"do": {"filter-bar"}, "bar": {"Nota", "Tamat"}})
}

func TestResetToTemplatePutsBackTheTemplatesBlocksFromTheCurrentColumns(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	opened, bar := ada.pageBlocks(path + "/view/")
	res, body := ada.get(path + "/blocks")
	for _, want := range []string{"Reset to Template", "Your own blocks and changes go", `name="do" value="reset"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}

	ada.editBlocks(path, url.Values{"do": {"remove"}, "block": {"0"}})
	ada.editBlocks(path, url.Values{"do": {"add"}, "kind": {"table"}})
	ada.editBlocks(path, barForm(url.Values{"block": {"4"}, "label": {"Belanja"}}))
	ada.editBlocks(path, url.Values{"do": {"filter-bar"}, "bar": {"Perkara"}})
	ada.editBlocks(path, url.Values{"do": {"reset"}})
	blocks, filters := ada.pageBlocks(path + "/view/")
	if labels(blocks) != labels(opened) || fmt.Sprint(filters) != fmt.Sprint(bar) {
		t.Fatalf("after Reset: %s %v, want %s %v", labels(blocks), filters, labels(opened), bar)
	}

	// A new Dataset without Kategori: Reset leaves out the blocks that need it.
	day := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	res, body = ada.postFile(path+"/dataset", "baru.xlsx", testutil.XLSX(t, []string{"Tarikh", "Perkara", "Jenis", "Jumlah"},
		[]any{day, "Gula", "Keluar", 30.0}, []any{day, "Jualan", "Masuk", 500.0}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	ada.editBlocks(path, url.Values{"do": {"reset"}})
	blocks, filters = ada.pageBlocks(path + "/view/")
	if strings.Contains(labels(blocks), "Spending by category") || !strings.Contains(labels(blocks), "Balance") || slices.Contains(filters, "Kategori") {
		t.Fatalf("Reset without Kategori: %s %v", labels(blocks), filters)
	}

	ada.switchLanguage("ms", path+"/blocks")
	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusOK, "Set semula ke Template")
}

func TestALockedBlockKeepsItsKindAndCalculationButMovesToOtherColumns(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	blocks, _ := ada.pageBlocks(path + "/view/")
	i := slices.IndexFunc(blocks, func(b lab.WorkBlock) bool { return b.Label == "Balance" })
	at := fmt.Sprint(i)
	res, body := ada.get(path + "/blocks?open=" + at)
	for _, want := range []string{`<fieldset class="field" disabled><legend>Kind`, `name="agg" disabled`,
		"You can still rename, move, resize or remove it, or point it at other columns.", `name="where"`, `name="minus"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}

	form := func(set url.Values) url.Values {
		f := url.Values{"do": {"save"}, "block": {at}, "label": {"Balance"}, "col": {"Jumlah"}, "where": {"Jenis"}, "minus": {"Jenis"}, "fcol": {""}}
		for k, v := range set {
			f[k] = v
		}
		return f
	}
	for _, f := range []url.Values{form(url.Values{"kind": {"bar"}}), form(url.Values{"agg": {"count"}}), form(url.Values{"where": {"Jumlah"}})} {
		res, body := ada.post(path+"/blocks", f)
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `class="up-err" role="alert"`) {
			t.Fatalf("%v = %d, want refused:\n%s", f, res.StatusCode, body)
		}
	}
	res, body = ada.post(path+"/blocks", form(url.Values{"kind": {"bar"}}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "kind and calculation stay as the Template made them")

	ada.editBlocks(path, form(url.Values{"label": {"Baki"}, "where": {"Kategori"}, "minus": {"Kategori"}}))
	blocks, _ = ada.pageBlocks(path + "/view/")
	b := blocks[i]
	if b.Label != "Baki" || !b.Locked || b.Kind != "card" || b.Value.Agg != "sum" || b.Value.Col != "Jumlah" ||
		fmt.Sprint(b.Where, b.Minus) != "[Kategori Masuk] [Kategori Keluar]" {
		t.Fatalf("the repointed Balance is %+v", b)
	}
}

func TestALockedBlockTakesNoFilter(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	blocks, _ := ada.pageBlocks(path + "/view/")
	at := fmt.Sprint(slices.IndexFunc(blocks, func(b lab.WorkBlock) bool { return b.Label == "Balance" }))
	res, body := ada.get(path + "/blocks?open=" + at)
	expectStatus(t, res, body, http.StatusOK, `name="where"`)
	if strings.Contains(body, `name="fcol"`) {
		t.Fatal("a locked block's form offers a filter")
	}
	res, body = ada.post(path+"/blocks", url.Values{"do": {"save"}, "block": {at}, "label": {"Balance"}, "col": {"Jumlah"},
		"where": {"Jenis"}, "minus": {"Jenis"}, "fcol": {"Kategori"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `class="up-err" role="alert"`)
}

func TestResetOnGeneralRebuildsFromTheCurrentDataset(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "senarai.xlsx", testutil.XLSX(t, []string{"Hari", "Nota"}, []any{day, "Buka"}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", url.Values{"template": {"general"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.postFile(path+"/dataset", "senarai.xlsx", testutil.XLSX(t, []string{"Hari", "Nota", "Kos"}, []any{day, "Buka", 12.5}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	ada.editBlocks(path, url.Values{"do": {"reset"}})
	blocks, _ := ada.pageBlocks(path + "/view/")
	if table := blocks[len(blocks)-1]; table.Kind != "table" || strings.Join(table.Cols, ",") != "Hari,Nota,Kos" {
		t.Fatalf("Reset on General gave %+v", blocks)
	}
}
