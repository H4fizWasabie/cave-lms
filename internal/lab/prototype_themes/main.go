// PROTOTYPE, throwaway: writes the HR Sample as a Work Dashboard with six cards into this folder, so the five
// Themes can be flipped through with ?variant=. Run: go run ./internal/lab/prototype_themes
package main

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

func main() {
	t := lab.TemplateByKey("hr")
	d, err := lab.Read(bytes.NewReader(t.Sample), t)
	if err != nil {
		panic(err)
	}
	count := &lab.Value{Agg: "count"}
	cfg := &lab.Config{
		FilterBar: []string{"Jabatan", "Status", "Jantina", "Tarikh Mula"},
		Blocks: []lab.WorkBlock{
			{Kind: "card", Label: "Headcount", Value: count},
			{Kind: "card", Label: "Active", Value: count, Filter: &lab.Filter{Col: "Status", Vals: []string{"Aktif"}}},
			{Kind: "card", Label: "Departments", Value: &lab.Value{Agg: "distinct", Col: "Jabatan"}},
			{Kind: "card", Label: "Positions", Value: &lab.Value{Agg: "distinct", Col: "Jawatan"}},
			{Kind: "card", Label: "Women", Value: count, Filter: &lab.Filter{Col: "Jantina", Vals: []string{"Perempuan"}}},
			{Kind: "card", Label: "Men", Value: count, Filter: &lab.Filter{Col: "Jantina", Vals: []string{"Lelaki"}}},
			{Kind: "bar", Label: "Headcount by department", Value: count, Group: "Jabatan"},
			{Kind: "bar", Label: "Status by department", Value: count, Group: "Jabatan", Split: "Status"},
			{Kind: "line", Label: "Joins per month", Value: count, X: "Tarikh Mula", By: "month", Wide: true},
			{Kind: "donut", Label: "Gender", Value: count, Group: "Jantina"},
			{Kind: "donut", Label: "Status split", Value: count, Group: "Status"},
			{Kind: "table", Label: "Staff", Cols: []string{"Nama", "Jabatan", "Jawatan", "Tarikh Mula", "Status", "Jantina"}, Sort: "Tarikh Mula", Limit: 10, Wide: true},
		},
	}
	index, data, err := lab.GenerateWork(t, "HR Sample", cfg, d, "data.json")
	if err != nil {
		panic(err)
	}
	dir := "internal/lab/prototype_themes"
	// The switcher: ?variant= picks the Theme before the Dashboard draws; arrows and the arrow keys cycle it.
	switcher := `<script>
  var THEMES = [['corporate','Corporate Executive'],['midnight','Midnight Intelligence'],['minimal','Minimal Insight'],['glass','Glass Vision'],['storytelling','Data Storytelling']];
  var cur = Math.max(0, THEMES.findIndex(function (t) { return t[0] === new URLSearchParams(location.search).get('variant'); }));
  dashboard.theme = THEMES[cur][0];
  function go(step) { location.search = '?variant=' + THEMES[(cur + step + THEMES.length) % THEMES.length][0]; }
  addEventListener('keydown', function (e) { if (e.target.closest('input,select,textarea')) return; if (e.key === 'ArrowLeft') go(-1); if (e.key === 'ArrowRight') go(1); });
  document.addEventListener('DOMContentLoaded', function () {
    var bar = document.createElement('div');
    bar.style.cssText = 'position:fixed;bottom:18px;left:50%;transform:translateX(-50%);z-index:9;display:flex;gap:.6rem;align-items:center;background:#111;color:#fff;font:600 13px system-ui;padding:.45rem .6rem;border-radius:99px;box-shadow:0 6px 24px rgba(0,0,0,.35)';
    bar.innerHTML = '<button id="pv">&larr;</button><span>PROTOTYPE ' + (cur + 1) + '/5: ' + THEMES[cur][1] + '</span><button id="nx">&rarr;</button>';
    bar.querySelectorAll('button').forEach(function (b) { b.style.cssText = 'all:unset;cursor:pointer;padding:.1rem .6rem;border-radius:99px;background:#333'; });
    document.body.append(bar);
    document.getElementById('pv').onclick = function () { go(-1); };
    document.getElementById('nx').onclick = function () { go(1); };
  });
</script>
`
	index = bytes.Replace(index, []byte("    fetch("), []byte("    </script>\n"+switcher+"  <script>\n    fetch("), 1)
	for name, b := range map[string][]byte{"index.html": index, "data.json": data, "charts.js": lab.ChartScript} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			panic(err)
		}
	}
}
