package app_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

var samePartReplace = regexp.MustCompile(`href="(/lab/dashboards/\d+/parts/\d+/replace)"[^>]*>Replace that Part<`)

func partCount(t *testing.T, ada *site) (n int) {
	t.Helper()
	if err := ada.db.QueryRow(`SELECT count(*) FROM dataset_parts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddingTheSameFileTwiceIsRefusedNamingThePartWithAWorkingReplaceButton(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	if n := len(replaceLinks(t, ada, path+"/data")); n != 2 {
		t.Fatalf("%d Parts, want 2", n)
	}

	// oktober.xlsx's bytes again, under another name.
	res, body := ada.postFile(path+"/dataset", "oktober-lagi.xlsx", shopColumns(t, "Gula", 80), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "oktober-lagi.xlsx", "oktober.xlsx", "exactly the same file", "Replace that Part", "Nothing changed")
	m := samePartReplace.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the refusal has no Replace that Part button: %s", body)
	}
	if want := replaceLinks(t, ada, path+"/data")[1]; m[1] != want {
		t.Fatalf("the button goes to %s, want the matching Part's %s", m[1], want)
	}
	expectAll(t, mustGet(t, ada, m[1]), "Replace Part", "oktober.xlsx")
	if n := partCount(t, ada); n != 2 {
		t.Fatalf("%d Parts after the refusal, want 2 and no pending one", n)
	}

	ada.switchLanguage("ms", path+"/data")
	_, body = ada.postFile(path+"/dataset", "oktober-lagi.xlsx", shopColumns(t, "Gula", 80), adding)
	expectAll(t, body, "fail yang sama sepenuhnya", "Gantikan Bahagian itu")
}

func TestTheSameFileIsStillRefusedAfterItsWorkbookWasDeleted(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	ada.forgetWorkbooks()

	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "exactly the same file", "Replace that Part")
	if n := partCount(t, ada); n != 2 {
		t.Fatalf("%d Parts, want 2", n)
	}
	var kept int
	if err := ada.db.QueryRow(`SELECT count(*) FROM workbooks`).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("%d Workbooks kept for the refused file, want none: %v", kept, err)
	}
}

func TestADifferentFileThatRepeatsNinetyPercentOfAPartsRowsIsAccepted(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	header := []any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}
	report := func(last string) []byte {
		var rows [][]any
		for i := 1; i <= 9; i++ {
			rows = append(rows, []any{day, fmt.Sprintf("Barang %d", i), "Dapur", "Keluar", float64(10 * i)})
		}
		return underTitleRows(t, header, append(rows, []any{day, last, "Dapur", "Keluar", 5.0})...)
	}
	res, body := ada.postFile(path+"/dataset", "september.xlsx", report("Beras"))
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)

	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", report("Gula"), adding)
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	if n := len(replaceLinks(t, ada, path+"/data")); n != 2 {
		t.Fatalf("%d Parts, want the second file added", n)
	}
}

func TestReplacingAPartWithAnotherPartsFileIsRefusedButWithItsOwnFileOrReplaceDataIsNot(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	links := replaceLinks(t, ada, path+"/data")

	// september.xlsx's bytes to replace oktober: the matching Part is the first one.
	res, body := ada.postFile(links[1], "september.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	if m := samePartReplace.FindStringSubmatch(body); m == nil || m[1] != links[0] {
		t.Fatalf("the refusal's button goes to %v, want %s: %s", m, links[0], body)
	}

	// A Part read again from its own file, as when the preview posts it with other choices, is not its own duplicate.
	res, body = ada.postFile(links[1], "oktober.xlsx", shopColumns(t, "Gula", 80))
	expectRedirect(t, res, body, path+"/data")

	// Replace data throws every Part away, so a file one of them came from is fine.
	res, body = ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectRedirect(t, res, body, path+"/data")
}

func TestAPartFromBeforeFingerprintsGetsItFromItsWorkbookInTheMigration(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	if err := app.MigrateDownTo(context.Background(), ada.db, 128); err != nil {
		t.Fatal(err)
	}
	ada.restart()

	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	if !strings.Contains(body, "exactly the same file") {
		t.Fatalf("a migrated Part's file is not recognised: %s", body)
	}
}
