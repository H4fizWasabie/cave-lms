package text

import (
	"strings"
	"testing"
)

// The Lab has one kind of Dashboard (ADR 0015): no text names the two former kinds, in either language.
func TestNoTextNamesTheFormerKindsOfDashboard(t *testing.T) {
	for key, e := range catalog {
		for _, s := range []string{e.en, e.ms} {
			for _, former := range []string{"practice dashboard", "work dashboard", "dashboard latihan", "dashboard kerja"} {
				if strings.Contains(strings.ToLower(s), former) {
					t.Errorf("%s names a former kind of Dashboard: %q", key, s)
				}
			}
		}
	}
}
