package text_test

import (
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func TestEveryTextHasBothLanguagesWithTheSameValues(t *testing.T) {
	if err := text.CheckCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestTextsFillInValuesAndPickSingularOrPlural(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{text.T("en", "header.logged-in-as", "Ada"), "Logged in as Ada"},
		{text.T("ms", "header.logged-in-as", "Ada"), "Log masuk sebagai Ada"},
		{text.N("en", "cavetext.snippet-count", 1), "1 Snippet"},
		{text.N("en", "cavetext.snippet-count", 3), "3 Snippets"},
		{text.N("ms", "cavetext.snippet-count", 3), "3 Snippet"},
		{text.T("en", "no.such.key"), "no.such.key"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestDatesUseTheLanguagesMonthNames(t *testing.T) {
	day := time.Date(2026, time.October, 22, 15, 0, 0, 0, time.UTC)
	if got := text.Date("ms", day); got != "22 Oktober 2026" {
		t.Errorf("Malay date = %q", got)
	}
	if got := text.Date("en", day); got != "22 October 2026" {
		t.Errorf("English date = %q", got)
	}
	if got := text.ShortDate("ms", day); got != "22 Okt 2026" {
		t.Errorf("Malay short date = %q", got)
	}
	if got := text.ShortDate("en", day); got != "22 Oct 2026" {
		t.Errorf("English short date = %q", got)
	}
}

func TestTheBrowsersFirstMalayOrEnglishIsChosenElseMalay(t *testing.T) {
	for header, want := range map[string]string{
		"":                        "ms",
		"en-GB,en;q=0.9":          "en",
		"ms-MY,ms;q=0.9,en;q=0.8": "ms",
		"fr-FR, EN-us;q=0.7, ms":  "en",
		"zh-CN,fr;q=0.8":          "ms",
		"english":                 "ms",
	} {
		if got := text.FromAcceptLanguage(header); got != want {
			t.Errorf("FromAcceptLanguage(%q) = %q, want %q", header, got, want)
		}
	}
}
