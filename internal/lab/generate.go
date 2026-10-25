package lab

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
)

//go:embed charts.js
var chartsJS []byte

// fonts is Inter at the two weights the Themes use (fonts/OFL.txt has its licence).
//
//go:embed fonts/inter-400.woff2 fonts/inter-600.woff2
var fonts embed.FS

// ChartScript is charts.js, Cave's own chart library: it draws and styles every Dashboard.
// It is served beside each generated index.html, which loads it by relative path.
// Inter is written into it as data URLs, so a Dashboard loads no font from anywhere, a Deployment's sandbox included.
var ChartScript = func() []byte {
	css := ""
	for _, w := range []string{"400", "600"} {
		b, err := fonts.ReadFile("fonts/inter-" + w + ".woff2")
		if err != nil {
			panic(err)
		}
		css += "@font-face{font-family:Inter;font-weight:" + w + ";font-display:swap;src:url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b) + ") format(\"woff2\")}"
	}
	return bytes.Replace(chartsJS, []byte("var FONTS = '';"), []byte("var FONTS = '"+css+"';"), 1)
}()

// Theme is how a Dashboard looks; charts.js holds each one's colours under its Key.
type Theme struct {
	Key, Name string
}

// Themes are the Themes a Learner can pick, the client's five, Corporate Executive first: it is every Dashboard's
// until its Learner picks another. Their names stay in English in both languages.
var Themes = []Theme{
	{"corporate", "Corporate Executive"},
	{"midnight", "Midnight Intelligence"},
	{"minimal", "Minimal Insight"},
	{"glass", "Glass Vision"},
	{"storytelling", "Data Storytelling"},
}

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

// Generate writes a Dashboard: index.html, which loads charts.js beside it and its data from dataURL and draws
// in theme (a Theme's key), and data.json, the Dataset as one object per row. Blocks that need a skipped blank are left out.
// Choices that do not fit the Dataset give a Problem, as does a blank cell in a column chosen for a
// required blank: the first such cell, by its Excel row.
func Generate(t *Template, name, theme string, c Choices, d *Dataset, dataURL string) (index, data []byte, err error) {
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
	for ri, row := range d.Rows {
		for ci, v := range row {
			if v == nil && required[d.Columns[ci].Name] {
				return nil, nil, emptyCell(d.rowNumber(ri), d.Columns[ci].Name)
			}
		}
	}

	blocks := t.included(c)
	var filters []string
	for _, k := range t.Filters {
		if c[k] != "" {
			filters = append(filters, k)
		}
	}

	return page(name, t.Question, map[string]any{"theme": theme, "columns": columns, "blocks": blocks, "filters": filters}, d, dataURL)
}

// GenerateWork writes a Work Dashboard as Generate writes a Practice one, drawing its own blocks and filter bar.
// Every column of the Dataset is offered to the blocks by its name.
func GenerateWork(t *Template, name, theme string, cfg *Config, d *Dataset, dataURL string) (index, data []byte, err error) {
	money := map[string]bool{}
	for _, col := range cfg.Money {
		money[col] = true
	}
	columns := map[string]chosenColumn{}
	for _, col := range d.Columns {
		columns[col.Name] = chosenColumn{Name: col.Name, Type: col.Type, Money: money[col.Name] && col.Type == Number}
	}
	return page(name, t.Question, map[string]any{"work": true, "theme": theme, "columns": columns, "blocks": cfg.Blocks, "filters": cfg.FilterBar}, d, dataURL)
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
