package lab

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
)

// ChartScript is charts.js, Cave's own chart library: it draws and styles every Dashboard.
// It is served beside each generated index.html, which loads it by relative path.
//
//go:embed charts.js
var ChartScript []byte

//go:embed dashboard.html
var dashboardHTML string

var dashboardPage = template.Must(template.New("dashboard").Parse(dashboardHTML))

// chosenColumn is a filled blank as the chart script sees it.
type chosenColumn struct {
	Name  string `json:"name"`
	Type  Type   `json:"type"`
	Money bool   `json:"money,omitempty"`
}

// Generate writes a Dashboard: index.html, which loads charts.js and data.json beside it, and
// data.json, the Dataset as one object per row. Blocks that need a skipped blank are left out.
// Choices that do not fit the Dataset give an error worded for the Learner.
func Generate(t *Template, name string, c Choices, d *Dataset) (index, data []byte, err error) {
	if t.Build != nil {
		t, c = t.Build(d)
	}
	types := map[string]Type{}
	for _, col := range d.Columns {
		types[col.Name] = col.Type
	}
	columns := map[string]chosenColumn{}
	for _, b := range t.Blanks {
		col, ok := c[b.Key]
		switch {
		case !ok && b.Required:
			return nil, nil, fmt.Errorf("Choose a column for %s.", b.Label)
		case !ok:
			continue
		case types[col] == "":
			return nil, nil, fmt.Errorf("%s: the Dataset has no column called %s.", b.Label, col)
		case types[col] != b.Type:
			return nil, nil, fmt.Errorf("%s needs a %s column, and %s holds %s.", b.Label, b.Type, col, types[col])
		}
		columns[b.Key] = chosenColumn{Name: col, Type: b.Type, Money: b.Money}
	}

	var blocks []Block
	for _, b := range t.Blocks {
		if b.Unless != "" && c[b.Unless] != "" {
			continue
		}
		needs := append([]string{b.Of, b.X, b.Group}, b.Needs...)
		ok := true
		for _, k := range needs {
			ok = ok && (k == "" || c[k] != "")
		}
		if ok {
			blocks = append(blocks, b)
		}
	}
	var filters []string
	for _, k := range t.Filters {
		if c[k] != "" {
			filters = append(filters, k)
		}
	}

	config, err := json.MarshalIndent(map[string]any{"columns": columns, "blocks": blocks, "filters": filters}, "    ", "  ")
	if err != nil {
		return nil, nil, err
	}
	var page bytes.Buffer
	// template.JS is safe here: encoding/json escapes <, >, & and U+2028/2029, so no text can end the script.
	err = dashboardPage.Execute(&page, map[string]any{"Name": name, "Config": template.JS(config)})
	if err != nil {
		return nil, nil, err
	}
	data, err = dataJSON(d)
	return page.Bytes(), data, err
}

// dataJSON is the Dataset as a JSON array with one object per row, keys in column order, one row per line.
func dataJSON(d *Dataset) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("[")
	for ri, row := range d.Rows {
		if ri > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n  {")
		for ci, v := range row {
			k, err := json.Marshal(d.Columns[ci].Name)
			if err != nil {
				return nil, err
			}
			val, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			if ci > 0 {
				b.WriteString(", ")
			}
			b.Write(k)
			b.WriteString(": ")
			b.Write(val)
		}
		b.WriteString("}")
	}
	b.WriteString("\n]\n")
	return b.Bytes(), nil
}
