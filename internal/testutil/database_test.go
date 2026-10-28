package testutil_test

import (
	"os"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

func TestEachDatabaseIsMigratedAndSeesNoOthersData(t *testing.T) {
	t.Parallel()
	first, _ := testutil.NewDatabase(t)
	second, _ := testutil.NewDatabase(t)
	if _, err := first.Exec("CREATE TABLE scratch (note text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec("INSERT INTO scratch VALUES ('first')"); err != nil {
		t.Fatal(err)
	}
	var seen bool
	if err := second.QueryRow("SELECT to_regclass('scratch') IS NOT NULL").Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen {
		t.Error("the second database sees the first's table")
	}
	var migrated bool
	if err := second.QueryRow("SELECT to_regclass('goose_db_version') IS NOT NULL").Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Error("the second database is not migrated")
	}
}
