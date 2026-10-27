package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
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

func labels(blocks []lab.WorkBlock) string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.Label)
	}
	return strings.Join(out, ", ")
}

// paused counts the blocks the Dashboard page at page (ending in /) keeps but leaves off: those adding up a column its
// Dataset lacks as a number.
func (s *site) paused(page string) int {
	s.t.Helper()
	_, body := s.get(page)
	m := pageConfig.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("%s has no config: %s", page, body)
	}
	var cfg struct {
		Columns map[string]struct{ Type lab.Type } `json:"columns"`
		Blocks  []lab.WorkBlock                    `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(m[1]), &cfg); err != nil {
		s.t.Fatal(err)
	}
	n := 0
	for _, b := range cfg.Blocks {
		if v := b.Value; v != nil && (v.Agg == "sum" || v.Agg == "avg") && cfg.Columns[v.Col].Type != lab.Number {
			n++
		}
	}
	return n
}

// Block editing and Reset are gone (ADR 0019): the Blocks tab is not found, and posting an edit changes nothing.
func TestTheBlocksTabIsGone(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.workReady()
	before, _ := ada.pageBlocks(path + "/view/")
	res, body := ada.get(path)
	if strings.Contains(body, `href="`+path+`/blocks"`) {
		t.Fatalf("the Dashboard still links its Blocks tab:\n%s", body)
	}
	res, body = ada.get(path + "/blocks")
	expectStatus(t, res, body, http.StatusNotFound, "")
	for _, form := range []url.Values{{"do": {"reset"}}, {"do": {"remove"}, "block": {"0"}}} {
		res, body = ada.post(path+"/blocks", form)
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	if after, _ := ada.pageBlocks(path + "/view/"); labels(after) != labels(before) {
		t.Fatalf("the blocks changed: %s, were %s", labels(after), labels(before))
	}
}
