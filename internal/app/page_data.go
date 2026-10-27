package app

import (
	"net/http"
	"strings"
)

// dataTab is the screen a Dashboard's Dataset is uploaded on: the data screen for a page Dashboard (ADR 0018), the
// Dataset tab for a built one. dataURL is its address.
func dataTab(d *dashboard) string {
	if d.Page != nil {
		return "data"
	}
	return "dataset"
}

// homeTab is the screen at a Dashboard's own address: the Page screen for a page Dashboard, the Dataset tab for a
// built one.
func homeTab(d *dashboard) string {
	if d.Page != nil {
		return "page"
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

// servePageData writes a page Dashboard's current Dataset at <link>/data.json, every Part's rows in order each with its
// Part date (see serveParts), or answers 404 without one. Any origin may read it, as the sandboxed page's origin is null; it is cached five
// minutes, so a new confirm, sleep and Takedown take effect soon.
func (a *App) servePageData(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if d.Parts == 0 && d.Columns == nil { // not uploaded yet; once the last Part is deleted, the columns are an empty list
		a.notFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	a.serveParts(w, r, d, "public, max-age=300")
}
