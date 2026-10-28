package text

import (
	"strings"
	"testing"
)

func withCatalog(t *testing.T, c map[string]entry) {
	t.Helper()
	old := catalog
	catalog = c
	t.Cleanup(func() { catalog = old })
}

func TestCheckCatalogNamesEveryProblem(t *testing.T) {
	withCatalog(t, map[string]entry{
		"ok":         {en: "Hi %s", ms: "Hai %s"},
		"reordered":  {en: "%s of %s", ms: "%[2]s daripada %[1]s"},
		"nolang":     {en: "Only English"},
		"mismatch":   {en: "%s and %s", ms: "%s"},
		"lonely.one": {en: "1 x", ms: "1 x"},
		"solo.other": {en: "%d x", ms: "%d x"},
		"pair.one":   {en: "1 y", ms: "1 y"},
		"pair.other": {en: "%d y", ms: "%d y"},
	})
	err := CheckCatalog()
	if err == nil {
		t.Fatal("want an error")
	}
	want := strings.Join([]string{
		"text catalog:",
		"lonely.one has no .other",
		"mismatch fills in different values in English and Malay",
		"nolang lacks a language",
		"solo.other has no .one",
	}, "\n")
	if err.Error() != want {
		t.Errorf("got\n%s\nwant\n%s", err, want)
	}
}

func TestCheckCatalogPassesAValidCatalog(t *testing.T) {
	withCatalog(t, map[string]entry{"ok": {en: "%s", ms: "%s"}})
	if err := CheckCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestVerbsNumbersPlainVerbsInArgumentOrder(t *testing.T) {
	// English fills name, count; Malay reorders them explicitly: same values.
	en := verbs("%s has %d")
	ms := verbs("%[2]d dimiliki %[1]s")
	if strings.Join(en, ",") != strings.Join(ms, ",") {
		t.Errorf("en %v != ms %v", en, ms)
	}
	if got := strings.Join(en, ","); got != "%[1]s,%[2]d" {
		t.Errorf("verbs = %q", got)
	}
	// A plain verb after an explicit one still takes the next counter value from 1.
	if got := strings.Join(verbs("%[2]d then %s"), ","); got != "%[1]s,%[2]d" {
		t.Errorf("mixed verbs = %q", got)
	}
	// Swapped types differ.
	if strings.Join(verbs("%s %d"), ",") == strings.Join(verbs("%[2]s %[1]d"), ",") {
		t.Error("different values compared equal")
	}
}

func TestHasFindsAKeyOrItsPluralForms(t *testing.T) {
	withCatalog(t, map[string]entry{
		"plain":       {en: "a", ms: "a"},
		"count.other": {en: "%d", ms: "%d"},
	})
	for key, want := range map[string]bool{"plain": true, "count": true, "count.one": false, "none": false} {
		if got := Has(key); got != want {
			t.Errorf("Has(%q) = %v, want %v", key, got, want)
		}
	}
}
