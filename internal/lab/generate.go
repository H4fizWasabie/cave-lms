package lab

import (
	"bytes"
	_ "embed"
	"encoding/json"
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

// Check gives a Problem for the first choice that does not fit the Dataset: a required blank with no column, or a
// column missing from the Dataset or of the wrong type.
func (t *Template) Check(c Choices, d *Dataset) error {
	types := map[string]Type{}
	for _, col := range d.Columns {
		types[col.Name] = col.Type
	}
	for _, b := range t.Blanks {
		col, ok := c[b.Key]
		switch {
		case !ok && b.Required:
			return Problem{"lab.err.choose-column", []any{b.Label}}
		case !ok:
		case types[col] == "":
			return Problem{"lab.err.no-such-column", []any{b.Label, col}}
		case types[col] != b.Type:
			return Problem{"lab.err.wrong-type", []any{b.Label, typeWord(b.Type), col, typeWord(types[col])}}
		}
	}
	return nil
}

// Generate writes a Dashboard: index.html, which loads charts.js beside it and its data from dataURL, and
// data.json, the Dataset as one object per row. Blocks that need a skipped blank are left out.
// Choices that do not fit the Dataset give a Problem; blank cells in a
// column chosen for a required blank give Errors, one per cell.
func Generate(t *Template, name string, c Choices, d *Dataset, dataURL string) (index, data []byte, err error) {
	if t.Build != nil {
		t, c = t.Build(d)
	}
	if err := t.Check(c, d); err != nil {
		return nil, nil, err
	}
	columns, required := map[string]chosenColumn{}, map[string]bool{}
	for _, b := range t.Blanks {
		col, ok := c[b.Key]
		if !ok {
			continue
		}
		columns[b.Key] = chosenColumn{Name: col, Type: b.Type, Money: b.Money}
		required[col] = required[col] || b.Required
	}
	var blanks Errors
	for ri, row := range d.Rows {
		for ci, v := range row {
			if v == nil && required[d.Columns[ci].Name] {
				blanks = append(blanks, emptyCell(d.rowNumber(ri), d.Columns[ci].Name))
			}
		}
	}
	if blanks != nil {
		return nil, nil, blanks
	}

	blocks := t.included(c)
	var filters []string
	for _, k := range t.Filters {
		if c[k] != "" {
			filters = append(filters, k)
		}
	}

	return page(name, t.Question, map[string]any{"columns": columns, "blocks": blocks, "filters": filters}, d, dataURL)
}

// GenerateWork writes a Work Dashboard as Generate writes a Practice one, drawing its own blocks and filter bar.
// Every column of the Dataset is offered to the blocks by its name.
func GenerateWork(t *Template, name string, cfg *Config, d *Dataset, dataURL string) (index, data []byte, err error) {
	money := map[string]bool{}
	for _, col := range cfg.Money {
		money[col] = true
	}
	columns := map[string]chosenColumn{}
	for _, col := range d.Columns {
		columns[col.Name] = chosenColumn{Name: col.Name, Type: col.Type, Money: money[col.Name] && col.Type == Number}
	}
	return page(name, t.Question, map[string]any{"work": true, "columns": columns, "blocks": cfg.Blocks, "filters": cfg.FilterBar}, d, dataURL)
}

// page writes index.html with config as the chart script's settings, and data.json.
func page(name, question string, config map[string]any, d *Dataset, dataURL string) (index, data []byte, err error) {
	settings, err := json.MarshalIndent(config, "    ", "  ")
	if err != nil {
		return nil, nil, err
	}
	var out bytes.Buffer
	// template.JS is safe here: encoding/json escapes <, >, & and U+2028/2029, so no text can end the script.
	// The same holds for dataURL, written as a JSON string so its slashes stay readable.
	dataJS, err := json.Marshal(dataURL)
	if err != nil {
		return nil, nil, err
	}
	err = dashboardPage.Execute(&out, map[string]any{"Name": name, "Question": question, "Config": template.JS(settings), "Data": template.JS(dataJS)})
	if err != nil {
		return nil, nil, err
	}
	data, err = dataJSON(d)
	return out.Bytes(), data, err
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
