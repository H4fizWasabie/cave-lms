package app

import (
	"net/url"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// columnUse is one column a Work Dashboard's blocks use, as an upload's preview shows it: matched by itself
// when the new Dataset has it by name and type, else a pick among Options or left out (its blocks pause).
type columnUse struct {
	lab.Use
	Fits    bool
	Options []string
}

// columnUses are the columns cfg's blocks use, each as Dataset d has it.
func columnUses(cfg *lab.Config, d *lab.Dataset) []columnUse {
	var out []columnUse
	for _, u := range cfg.Uses() {
		out = append(out, columnUse{Use: u, Fits: u.Fits(d), Options: u.Options(d)})
	}
	return out
}

// allFit is whether d has every column cfg's blocks use, by name and type.
func allFit(cfg *lab.Config, d *lab.Dataset) bool {
	return missingUses(columnUses(cfg, d)) == 0
}

// missingUses counts the uses d does not fit.
func missingUses(uses []columnUse) int {
	n := 0
	for _, u := range uses {
		if !u.Fits {
			n++
		}
	}
	return n
}

// withPicks is cfg and the Template choices c with the Learner's pick for each column cfg's blocks use that d
// lacks or has of another type, posted as use:<column>; a column left out, or a pick d lacks, stays as it is.
func withPicks(cfg *lab.Config, c lab.Choices, d *lab.Dataset, form url.Values) (*lab.Config, lab.Choices) {
	m := map[string]string{}
	for _, u := range columnUses(cfg, d) {
		if to := form.Get("use:" + u.Name); !u.Fits && to != "" && (lab.Use{Name: to, Type: u.Type}).Fits(d) {
			m[u.Name] = to
		}
	}
	out := *cfg
	out.Rename(m)
	return &out, c.Rename(m)
}
