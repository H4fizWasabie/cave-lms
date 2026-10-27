package lab

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"slices"
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

// ThemeByKey is the Theme with key, and whether Cave offers one.
func ThemeByKey(key string) (Theme, bool) {
	i := slices.IndexFunc(Themes, func(t Theme) bool { return t.Key == key })
	if i < 0 {
		return Theme{}, false
	}
	return Themes[i], true
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

// Page writes a Dashboard's index.html, which loads charts.js beside it and its data from dataURL and draws its
// blocks and filter bar in theme (a Theme's key). Every one of the Dataset's columns is offered to the blocks by
// its name. It needs only the columns, so a page costs the same whatever the Dataset's size.
func Page(name, theme string, cfg *Config, columns []Column, dataURL string) ([]byte, error) {
	money := map[string]bool{}
	for _, col := range cfg.Money {
		money[col] = true
	}
	chosen := map[string]chosenColumn{}
	for _, col := range columns {
		chosen[col.Name] = chosenColumn{Name: col.Name, Type: col.Type, Money: money[col.Name] && col.Type == Number}
	}
	settings, err := json.MarshalIndent(map[string]any{"theme": theme, "columns": chosen, "blocks": cfg.Blocks, "filters": cfg.FilterBar}, "    ", "  ")
	if err != nil {
		return nil, err
	}
	// template.JS is safe here: encoding/json escapes <, >, & and U+2028/2029, so no text can end the script.
	// The same holds for dataURL, written as a JSON string so its slashes stay readable.
	dataJS, err := json.Marshal(dataURL)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = dashboardPage.Execute(&out, map[string]any{"Name": name, "Config": template.JS(settings), "Data": template.JS(dataJS)})
	return out.Bytes(), err
}

// PartDateKey is the key every row of data.json carries: the date of the Part the row is from.
const PartDateKey = "Part date"

// PartRows is a Part as stored, and as data.json serves it: one JSON object per row, keys in column order and the
// Part's date last, one row per line, rows separated by ",\n" and no brackets around them, so the Parts of a Dataset
// join into one array by writing their rows between "[\n" and "\n]\n" with ",\n" between Parts.
func PartRows(d *Dataset, date string) ([]byte, error) {
	var b bytes.Buffer
	day, err := json.Marshal(date)
	if err != nil {
		return nil, err
	}
	for ri, row := range d.Rows {
		if ri > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  {")
		for ci, v := range row {
			k, err := json.Marshal(d.Columns[ci].Name)
			if err != nil {
				return nil, err
			}
			val, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			b.Write(k)
			b.WriteString(": ")
			b.Write(val)
			b.WriteString(", ")
		}
		b.WriteString(`"` + PartDateKey + `": `)
		b.Write(day)
		b.WriteString("}")
	}
	return b.Bytes(), nil
}

// PartDataset reads a Part as PartRows wrote it back into a Dataset with the given columns, without the Part date.
func PartDataset(data []byte, cols []Column) (*Dataset, error) {
	var objects []map[string]any
	if err := json.Unmarshal(append(append([]byte("["), data...), ']'), &objects); err != nil {
		return nil, err
	}
	d := &Dataset{Columns: cols, Rows: make([][]any, len(objects))}
	for i, o := range objects {
		row := make([]any, len(cols))
		for j, c := range cols {
			row[j] = o[c.Name]
		}
		d.Rows[i] = row
	}
	return d, nil
}
