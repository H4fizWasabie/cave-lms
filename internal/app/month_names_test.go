package app_test

import (
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// monthNamedReport is augustReport with BULAN written as a month name, as the real August export does.
func monthNamedReport(t *testing.T, month any, schools ...[2]string) []byte {
	t.Helper()
	var rows [][]any
	for _, s := range schools {
		rows = append(rows, []any{"JOHOR", "Batu Pahat", s[0], s[1], "RENDAH", "SK", "T1", "1 Amanah", 2026.0, month, "31/08/2026", 120.0, 108.0, 12.0, 90.0})
	}
	return testutil.XLSX(t, augustColumns, rows...)
}
