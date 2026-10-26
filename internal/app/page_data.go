package app

import (
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// dataTab is the screen a Dashboard's Dataset is uploaded on: the data screen for a page Dashboard (ADR 0018), the
// Dataset tab for an engine one. dataURL is its address.
func dataTab(d *dashboard) string {
	if d.Page != nil {
		return "data"
	}
	return "dataset"
}

func dataURL(d *dashboard) string {
	if d.Page != nil {
		return dashboardURL(d) + "/data"
	}
	return dashboardURL(d)
}

// ColumnList is the pending Dataset's column names, for the data screen's preview.
func (p *pendingDataset) ColumnList() string {
	names := make([]string, len(p.Dataset.Columns))
	for i, c := range p.Dataset.Columns {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// servePageData writes a page Dashboard's current Dataset at <link>/data.json, in the rows format of lab.DataJSON,
// or answers 404 without one. Any origin may read it, as the sandboxed page's origin is null; it is cached five
// minutes, so a new confirm, sleep and Takedown take effect soon.
func (a *App) servePageData(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if d.Key == "" {
		a.notFound(w, r)
		return
	}
	f, ok := a.datasetFiles.get(d.Key)
	if !ok || f.data == nil {
		if err := a.readDataset(r.Context(), d); err != nil {
			a.serverError(w, r, err)
			return
		}
		data, err := lab.DataJSON(d.Dataset)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		f = datasetFiles{columns: d.Dataset.Columns, data: data}
		if !ok {
			a.datasetFiles.put(d.Key, f)
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(f.data)
}
