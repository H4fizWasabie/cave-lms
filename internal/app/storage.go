package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The Storage allowance (GLOSSARY.md): one rule for every upload that adds bytes, and for the Lab's "used". What a
// Learner keeps counts: their Parts and Pending Parts, Workbooks and pages, the earlier pages kept to restore included.
// Submissions do not. What an upload replaces counts as free as soon as it is uploaded, before the Learner confirms it.
// ponytail: that is why the store can briefly hold up to about twice the allowance, bounded by one Pending Part per
// Dashboard; refusing those uploads would refuse ones that fit once confirmed. Only a Pending Part is held on that
// credit: a Workbook that fits only on it goes again if the upload comes to nothing (keepWorkbook, dropBorrowed).
//
// A transaction that adds bytes calls reserve as its first statement, before any other row lock, so the Learner is
// always locked first and two uploads cannot deadlock. Deletes only free bytes and do not call it.

// queryer is a *sql.DB or a *sql.Tx, for reads that may run inside a caller's transaction.
type queryer interface {
	rowQueryer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// storageFull is the error of an upload (or confirm) that would take the Learner over their Storage allowance, with
// what the message names. storageMessage writes it.
type storageFull struct {
	Size      int64 // the file's size
	Used      int64 // what the Learner keeps, before the upload
	Pending   int64 // the part of Used that is Pending Parts
	Allowance int64
}

func (*storageFull) Error() string { return "over the Storage allowance" }

// storageMessage is the message for err in lang, or "" when err is not a storageFull. Every screen that refuses an
// upload for storage shows it; when Pending Parts are part of what is used, a second line says how much.
func storageMessage(lang string, err error) string {
	var full *storageFull
	if !errors.As(err, &full) {
		return ""
	}
	msg := text.T(lang, "lab.err.storage-full", sizeText(full.Size), sizeText(full.Used), sizeText(full.Allowance))
	if full.Pending > 0 {
		msg += "\n" + text.T(lang, "lab.err.storage-full-pending", sizeText(full.Pending))
	}
	return msg
}

// storageUsedSQL is an SQL expression for the bytes a Learner keeps against the Storage allowance, learner being an SQL
// expression for their id: the current and Pending Parts of their Dashboards (and any earlier Dataset from before Parts,
// until their Dashboard's data next changes) by saved size, their pages with the earlier ones kept, and their Workbooks.
func storageUsedSQL(learner string) string {
	return `((SELECT COALESCE(sum(ds.saved_size), 0) FROM dataset_parts ds JOIN dashboards owner ON owner.id = ds.dashboard_id
			WHERE owner.learner_id = ` + learner + `) +
		(SELECT COALESCE(sum(pg.size), 0) FROM dashboard_pages pg JOIN dashboards pg_owner ON pg_owner.id = pg.dashboard_id
			WHERE pg_owner.learner_id = ` + learner + `) +
		(SELECT COALESCE(sum(wb.size), 0) FROM workbooks wb WHERE wb.learner_id = ` + learner + `))`
}

// storageUsed is the bytes the Learner keeps against the Storage allowance, by storageUsedSQL.
func (a *App) storageUsed(ctx context.Context, learnerID int64) (used int64, err error) {
	err = a.db.QueryRowContext(ctx, `SELECT `+storageUsedSQL("$1"), learnerID).Scan(&used)
	return used, err
}

// storagePendingSQL is an SQL expression for the part of storageUsedSQL that is the Learner's Pending Parts.
func storagePendingSQL(learner string) string {
	return `(SELECT COALESCE(sum(p.saved_size), 0) FROM dataset_parts p JOIN dashboards owner ON owner.id = p.dashboard_id
		WHERE owner.learner_id = ` + learner + ` AND p.state = 'pending')`
}

// replaced is the bytes an upload replaces, which count as free; nil replaces none.
type replaced func(ctx context.Context, q queryer) (int64, error)

// replacedParts is what an upload to Dashboard dashboardID (0 for one not made yet) replaces: its own Pending Part, and
// the Parts a confirm of kind deletes (the Dataset module's Frees; replaces is the Part a ReplacePart one replaces).
// A confirm of that Pending Part replaces the same.
func replacedParts(dashboardID int64, kind dataset.Kind, replaces int64) replaced {
	return func(ctx context.Context, q queryer) (int64, error) {
		freed, err := dataset.Frees(ctx, q, dashboardID, kind, replaces)
		if err != nil {
			return 0, err
		}
		var n int64
		for _, f := range freed {
			n += f.Size
		}
		var pending int64
		err = q.QueryRowContext(ctx, `SELECT COALESCE(sum(saved_size), 0) FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending'`, dashboardID).Scan(&pending)
		return n + pending, err
	}
}

// replacedPage is what a new page for Dashboard dashboardID replaces: the earlier pages past the keepEarlier-1 that stay,
// as the current page turns earlier.
func replacedPage(dashboardID int64) replaced {
	return func(ctx context.Context, q queryer) (n int64, err error) {
		err = q.QueryRowContext(ctx, `SELECT COALESCE(sum(size), 0) FROM (SELECT size FROM dashboard_pages
			WHERE dashboard_id = $1 AND NOT current ORDER BY uploaded_at DESC, id DESC OFFSET $2) pushed`, dashboardID, keepEarlier-1).Scan(&n)
		return n, err
	}
}

// reserve is the Storage allowance's check, called first in the transaction tx of an upload that adds size bytes for the
// Learner: it locks the Learner, so uploads and confirms take turns and none slips under the allowance, then refuses with
// a *storageFull what would take them over it (see checkStorage). The Admin has no allowance: nothing is locked or refused.
func reserve(ctx context.Context, tx *sql.Tx, acc labAccess, learnerID, size int64, replaces replaced) error {
	if acc.CountedPeriod() == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learnerID); err != nil {
		return err
	}
	return checkStorage(ctx, tx, acc, learnerID, size, replaces)
}

// checkStorage is whether size more bytes fit the Learner's Storage allowance with replaces counted as free: nil, or a
// *storageFull. It locks nothing, so outside reserve it only tells what would happen.
func checkStorage(ctx context.Context, q queryer, acc labAccess, learnerID, size int64, replaces replaced) error {
	period := acc.CountedPeriod()
	if period == nil {
		return nil
	}
	full := &storageFull{Size: size, Allowance: period.Storage()}
	if err := q.QueryRowContext(ctx, `SELECT `+storageUsedSQL("$1")+`, `+storagePendingSQL("$1"), learnerID).Scan(&full.Used, &full.Pending); err != nil {
		return err
	}
	var free int64
	if replaces != nil {
		var err error
		if free, err = replaces(ctx, q); err != nil {
			return err
		}
	}
	if full.Used+size-free > full.Allowance {
		return full
	}
	return nil
}

// partReserve is the Dataset module's Upload reserve for a pending Part on Dashboard dashboardID (0 for one not made
// yet), the Learner's and acc's.
func partReserve(acc labAccess, learnerID, dashboardID int64, kind dataset.Kind, replaces int64) dataset.Reserve {
	return func(ctx context.Context, tx *sql.Tx, bytes int64) error {
		return reserve(ctx, tx, acc, learnerID, bytes, replacedParts(dashboardID, kind, replaces))
	}
}

// confirmReserve is the Dataset module's Confirm reserve for Dashboard dashboardID's pending Part. That Part is in what
// is used already and is among what replacedParts frees, so the confirm adds it once.
func confirmReserve(acc labAccess, learnerID, dashboardID int64) dataset.ConfirmReserve {
	return func(ctx context.Context, tx *sql.Tx, p *dataset.Pending) error {
		return reserve(ctx, tx, acc, learnerID, p.Size, replacedParts(dashboardID, p.Kind, p.ReplacedPart))
	}
}
