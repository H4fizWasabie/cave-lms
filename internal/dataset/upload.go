package dataset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// ErrStore is the error when the store refuses or cannot give a Part's rows. The error says which.
var ErrStore = errors.New("the store refused the Dataset")

// SameFileError is Upload's and SameFile's refusal of the exact file of a current Part (ADR 0022): the Part it is the
// same file as.
type SameFileError struct {
	ID   int64
	File string
	Date string // the Part date
}

func (e *SameFileError) Error() string { return "exactly the same file as Part " + e.File }

// MissingColumnsError is Upload's refusal of an upload that lacks columns the Dataset has, after matching by name and by
// the Learner's picks, and also of a pick of one of the file's columns for two of the Dataset's (Problem.Key says
// which). Unused are the file's columns the Dataset lacks and no pick or name matched: what the Learner may pick from.
type MissingColumnsError struct {
	Missing []string
	Unused  []string
	Problem lab.Problem
}

func (e *MissingColumnsError) Error() string { return e.Problem.Error() }

// Preview is what an upload showed the Learner besides its rows: Clean-up's result and what joining the Dataset's Parts
// found. It is kept as JSON with the pending Part, and the preview, the next upload and the confirm read it back. The
// embedded Clean-up result holds the rows the Part will keep.
type Preview struct {
	lab.CleanUp                // without its Dataset in the JSON
	Settings     lab.Settings  `json:"settings"`               // the remembered part of the settings Clean-up read with
	Columns      []lab.Column  `json:"columns"`                // the Part's columns, in the order its stored rows read back
	Added        *Addition     `json:"added,omitempty"`        // an Add to data; nil for one that replaces the data
	Replaces     *Replacement  `json:"replaces,omitempty"`     // a Replace of one Part of several; nil for an Add or a Replace of the data
	FromWorkbook bool          `json:"fromWorkbook,omitempty"` // read from a Workbook the Learner picked, so the preview posts its choices without a file
	HeaderTyped  bool          `json:"headerTyped,omitempty"`  // the Learner typed the header row, so the recipe of a first Part keeps it
	Mapped       []lab.Mapping `json:"mapped,omitempty"`       // an Add or a Replace of a Part: the upload's columns read as the Dataset's
	Unused       []string      `json:"unused,omitempty"`       // and the upload's columns left over after that, dropped
}

// Addition is what an Add to data put together: the upload's rows and the rows the Dataset had before them.
type Addition struct {
	Rows int `json:"rows"`
	Had  int `json:"had"`
}

// Total is the Dataset's rows once the Add is confirmed.
func (a Addition) Total() int { return a.Rows + a.Had }

// Replacement is what a Replace of one Part put together: the Part it replaces and the rows the other Parts hold.
type Replacement struct {
	Part int64  `json:"part"`
	File string `json:"file"`
	Rows int    `json:"rows"` // the replaced Part's rows
	Had  int    `json:"had"`  // the other Parts' rows
}

// Total is the Dataset's rows once the Replace is confirmed, given the new Part's rows.
func (r Replacement) Total(rows int) int { return rows + r.Had }

// Pending is the Dashboard's pending Part as Upload kept it, for the preview. Its Dataset holds the rows it stored,
// without the Part date.
type Pending struct {
	Preview
	ID           int64
	Key          string // where its rows are stored
	File         string
	Size         int64 // saved size, which counts against the Storage allowance once confirmed
	Workbook     int64 // the Workbook it was read from, 0 for none or once that Workbook is deleted
	Kind         Kind
	ReplacedPart int64 // the Part a ReplacePart one replaces, else 0
}

// Uploaded is an upload for Upload: the file, what Clean-up read from it and what the Learner chose.
type Uploaded struct {
	Kind     Kind
	Replaces int64             // the ID of the current Part a ReplacePart upload replaces
	File     string            // the file's name
	Sum      [sha256.Size]byte // the SHA-256 of the file's bytes
	Workbook int64             // the Workbook the file was kept as, 0 for none
	Read     *lab.CleanUp      // Clean-up's result
	Settings lab.Settings      // the settings Clean-up read with; its Recipe is ignored, the Dataset's is used
	Picks    map[string]string // the Learner's picks: a Dataset column to the file's column to read as it, "" for none
	// FromWorkbook says the file is a Workbook the Learner picked, so the preview posts its choices without a file.
	FromWorkbook bool
}

// Reserve checks that an upload adding bytes fits the Learner's Storage allowance, in the transaction tx that keeps the
// Part, and returns the error that refuses it. Upload calls it first in the transaction, before any row lock, so a
// reserve that locks the Learner always takes that lock first (#463).
type Reserve func(ctx context.Context, tx *sql.Tx, bytes int64) error

// state is the Dashboard's Dataset as an upload finds it.
type state struct {
	columns  []lab.Column
	settings lab.Settings // what the Dashboard remembers, with the recipe once the Dataset has Parts
	parts    int          // current Parts
	rows     int          // their rows in all
	page     bool         // the Dashboard has its Learner's own page, so no row limit
}

func (d *Dataset) load(ctx context.Context, dashboardID int64) (*state, error) {
	var st state
	var columns, settings []byte
	err := d.db.QueryRowContext(ctx, `SELECT dataset_columns, upload_settings,
			(SELECT count(*) FROM dataset_parts WHERE dashboard_id = dashboards.id AND state = 'current'),
			(SELECT COALESCE(sum(row_count), 0) FROM dataset_parts WHERE dashboard_id = dashboards.id AND state = 'current'),
			EXISTS (SELECT 1 FROM dashboard_pages WHERE dashboard_id = dashboards.id AND current)
		FROM dashboards WHERE id = $1`, dashboardID).Scan(&columns, &settings, &st.parts, &st.rows, &st.page)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(settings, &st.settings); err != nil {
		return nil, err
	}
	if columns != nil {
		if err := json.Unmarshal(columns, &st.columns); err != nil {
			return nil, err
		}
	}
	return &st, nil
}

// recipe is how an upload of kind is read: the Dataset's recipe, with its columns and the Learner's picks, for an Add or a
// Replace of one Part of a Dataset that has Parts; nil for the first Part and Replace data, which are read as ever.
func (st *state) recipe(kind Kind) *lab.Recipe {
	r := st.settings.Recipe
	if kind == ReplaceData || r == nil || st.parts == 0 {
		return nil
	}
	return &lab.Recipe{Sheet: r.Sheet, HeaderRow: r.HeaderRow, HeaderTyped: r.HeaderTyped, Columns: st.columns, MonthColumns: slices.Clone(r.MonthColumns), Map: maps.Clone(r.Map)}
}

// Recipe is the recipe the Dashboard's next upload of kind is read with, to give Clean-up: the sheet found by name and
// the header row if the Learner typed it, the Dataset's columns, month columns and the Learner's remembered picks. It is
// nil when the upload is read as a first Part is: Replace data, and a Dataset with no Part.
func (d *Dataset) Recipe(ctx context.Context, dashboardID int64, kind Kind) (*lab.Recipe, error) {
	st, err := d.load(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	return st.recipe(kind), nil
}

// SameFile reports with a *SameFileError that sum is the exact file of a current Part of the Dashboard that an upload of
// kind may not repeat: any but the Part replaces (0 for none). Replace data deletes every Part, so it may repeat one. Rows
// that merely repeat a Part's never match (ADR 0022). Upload checks it too; the app asks first to refuse before it keeps
// the file anywhere.
func (d *Dataset) SameFile(ctx context.Context, dashboardID int64, kind Kind, replaces int64, sum [sha256.Size]byte) error {
	if kind == ReplaceData {
		return nil
	}
	same := SameFileError{}
	err := d.db.QueryRowContext(ctx, `SELECT id, file_name, part_date::text FROM dataset_parts
		WHERE dashboard_id = $1 AND state = 'current' AND sha256 = $2 AND id <> $3 ORDER BY position LIMIT 1`,
		dashboardID, sum[:], replaces).Scan(&same.ID, &same.File, &same.Date)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return &same
}

// Upload reads the upload into the Dashboard's Dataset and keeps it as the Dashboard's pending Part, replacing any
// earlier one, and returns it for the preview. The Part is read as ADR 0022 says:
//   - An Add on a Dataset with no Part is Replace data. The first Part and Replace data keep Clean-up's result as it is.
//   - An Add or a Replace of one Part is put in the Dataset's columns: extra columns are dropped and listed; a missing
//     one is read from a file column whose name is equal ignoring case, spaces, underscores and dashes (a note), or the
//     one the Learner picked (u.Picks), which the recipe remembers; any still missing refuses the upload with a
//     *MissingColumnsError. Replace data forgets the picks with the recipe. Other refusals are a lab.Problem.
//   - A Dataset with row totals limited (a Dashboard Cave builds) refuses a total past the limit; one with the Learner's
//     own page has none.
//   - Month names in a month column are read as numbers. A Dataset recorded before month columns existed has them found
//     once, by reading its Parts, and its recipe records them.
//   - The exact file of a current Part is refused with a *SameFileError; a Replace of one Part may repeat that Part's.
//   - A Part with no date from Clean-up is dated the day of the upload, by the module's clock.
//
// A Replace of a Part that is not a current Part of the Dashboard is ErrPartGone. A store that refuses is ErrStore.
// reserve runs first in the transaction that keeps the Part; its error refuses the upload, which leaves no pending Part
// of the Upload's and no rows in the store.
func (d *Dataset) Upload(ctx context.Context, dashboardID int64, u Uploaded, reserve Reserve) (*Pending, error) {
	st, err := d.load(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	kind := u.Kind
	if kind == Add && st.parts == 0 {
		kind = ReplaceData
	}
	var replacing *Replacement
	if kind == ReplacePart {
		replacing = &Replacement{Part: u.Replaces}
		err := d.db.QueryRowContext(ctx, `SELECT file_name, row_count FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`,
			u.Replaces, dashboardID).Scan(&replacing.File, &replacing.Rows)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPartGone
		}
		if err != nil {
			return nil, err
		}
	} else {
		u.Replaces = 0
	}
	if err := d.SameFile(ctx, dashboardID, kind, u.Replaces, u.Sum); err != nil {
		return nil, err
	}

	pv := Preview{CleanUp: *u.Read, Settings: u.Settings, FromWorkbook: u.FromWorkbook, HeaderTyped: u.Settings.HeaderRow > 0}
	pv.Changes = slices.Clone(pv.Changes)
	pv.Settings.Recipe = nil
	if recipe := st.recipe(kind); kind != ReplaceData {
		pv.Settings.Recipe = recipe
		if recipe != nil { // the Learner's sheet pick is for this Part only
			pv.Settings.Sheet, pv.Settings.Alone = st.settings.Sheet, st.settings.Alone
		}
		had := st.rows
		if replacing != nil {
			had -= replacing.Rows
		}
		if recipe != nil {
			switch {
			case had == 0:
				// Nothing else is in the Dataset: the new Part's own values decide, as for a first Part, on confirm.
				recipe.MonthColumns = numberColumns(st.columns)
			case recipe.MonthColumns == nil:
				// A Dataset from before month columns were kept: found once from the Parts it keeps, saved with the
				// recipe on confirm.
				if recipe.MonthColumns, err = d.findMonthColumns(ctx, dashboardID, st.columns, u.Replaces); err != nil {
					return nil, err
				}
			}
		}
		if err := pv.join(st, replacing, had, u.Picks); err != nil {
			return nil, err
		}
	}

	pv.Columns = pv.Dataset.Columns
	if pv.PartDate == "" { // the Excel names no date: the upload day
		pv.PartDate, pv.DateFrom, pv.DateCell = d.now().UTC().Format(time.DateOnly), lab.DateFromUpload, ""
	}
	rows, err := lab.PartRows(pv.Dataset, pv.PartDate)
	if err != nil {
		return nil, err
	}
	cleanup, err := json.Marshal(pv)
	if err != nil {
		return nil, err
	}
	key, err := NewKey()
	if err != nil {
		return nil, err
	}
	if err := d.store.Put(ctx, key, rows); err != nil {
		return nil, fmt.Errorf("%w: put Part %s: %v", ErrStore, key, err)
	}
	p := &Pending{Preview: pv, Key: key, File: u.File, Workbook: u.Workbook, Size: int64(len(rows)), Kind: kind, ReplacedPart: u.Replaces}
	if err := d.keep(ctx, dashboardID, p, u, cleanup, len(pv.Dataset.Rows), reserve); err != nil {
		// A failed delete leaves only an unused object.
		// ponytail: not logged, as the module has no logger; the object is lost until the store is swept.
		_ = d.store.Delete(context.WithoutCancel(ctx), key)
		return nil, err
	}
	return p, nil
}

// keep checks the reserve and then replaces the Dashboard's pending Part with p, in one transaction.
func (d *Dataset) keep(ctx context.Context, dashboardID int64, p *Pending, u Uploaded, cleanup []byte, rows int, reserve Reserve) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The reserve comes first, before any row is locked: it may lock the Learner, who is always locked first (#463).
	if err := reserve(ctx, tx, p.Size); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending'`, dashboardID); err != nil {
		return err
	}
	var workbook, replaces any // NULL for a Part with no Workbook, and for one that replaces no Part
	if u.Workbook != 0 {
		workbook = u.Workbook
	}
	if p.Kind == ReplacePart {
		replaces = p.ReplacedPart
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, cleanup, workbook_id, part_date, date_source, sha256, upload_kind, replaces_part_id)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		dashboardID, p.Key, p.File, p.Size, rows, cleanup, workbook, p.PartDate, p.DateFrom, u.Sum[:], string(p.Kind), replaces).Scan(&p.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// join puts the upload's rows (pv.Dataset) in the Dataset's columns, as an Add does or, with replacing, a Replace of that
// Part does, and records in pv what it put together: the rows, the changes it made and the row totals. had is the rows
// the Dataset keeps besides the upload's. A Problem says why they do not fit; for the missing columns it is a
// *MissingColumnsError with the file's columns that are left. The Learner's picks map the upload's columns to the
// Dataset's. A Replace leaves out the rows of the Part it replaces.
func (pv *Preview) join(st *state, replacing *Replacement, had int, picks map[string]string) error {
	limit := lab.MaxCleanRows
	if st.page {
		limit = 0
	}
	var chosen map[string]string
	var monthCols []string
	var twice error
	if recipe := pv.Settings.Recipe; recipe != nil {
		monthCols = recipe.MonthColumns
		lacking, spare := lab.ColumnChanges(st.columns, pv.Dataset.Columns)
		chosen, twice = mapChoices(recipe.Map, picks, lacking, spare)
		recipe.Map = chosen // what the recipe remembers once the Part is confirmed
	}
	mapped, mappings, mapChanges := lab.MapColumns(st.columns, pv.Dataset, chosen)
	missing, unused := lab.ColumnChanges(st.columns, mapped.Columns)
	pv.Unused = unused
	if twice != nil {
		return &MissingColumnsError{Missing: missing, Unused: unused, Problem: twice.(lab.Problem)}
	}
	more, changes, err := lab.AddPart(st.columns, had, mapped, limit, monthCols)
	var problem lab.Problem
	if errors.As(err, &problem) {
		switch {
		case problem.Key == "lab.err.add-columns":
			return &MissingColumnsError{Missing: missing, Unused: unused, Problem: problem}
		case replacing != nil && problem.Key == "lab.err.add-too-many-rows":
			return lab.Problem{Key: "lab.err.replace-too-many-rows", Args: problem.Args}
		}
	}
	if err != nil {
		return err
	}
	if replacing != nil {
		pv.Replaces = &Replacement{Part: replacing.Part, File: replacing.File, Rows: replacing.Rows, Had: had}
	} else {
		pv.Added = &Addition{Rows: len(more.Rows), Had: st.rows}
	}
	pv.Dataset, pv.Mapped = more, mappings
	pv.Changes = append(append(pv.Changes, mapChanges...), changes...)
	return nil
}

// mapChoices is the recipe's map of the upload's columns to the Dataset's with the Learner's picks: a Dataset column to
// the upload's column to read as it, or "" to read none as it. Only a Dataset column in missing, the ones the upload
// lacks, can be picked for, and only by a column in unused, the upload's that the Dataset lacks; any other pick is
// dropped. A pick replaces what the recipe had for that Dataset column; the rest stays, as a file with other headers has
// no use for it. An upload's column picked for two Dataset columns is picked for neither, and the Problem names the
// three.
func mapChoices(saved, picks map[string]string, missing, unused []string) (map[string]string, error) {
	out := maps.Clone(saved)
	chosenFor := map[string]string{} // an upload's column to the Dataset column it was picked for
	var twice error
	for _, column := range slices.Sorted(maps.Keys(picks)) {
		if !slices.Contains(missing, column) {
			continue
		}
		file := strings.TrimSpace(picks[column])
		if file != "" && !slices.Contains(unused, file) {
			continue
		}
		for from, to := range out {
			if strings.EqualFold(to, column) {
				delete(out, from)
			}
		}
		if file == "" {
			continue
		}
		if other, ok := chosenFor[file]; ok {
			if twice == nil {
				twice = lab.Problem{Key: "lab.err.map-twice", Args: []any{file, other, column}}
			}
			delete(out, file)
			continue
		}
		chosenFor[file] = column
		if out == nil {
			out = map[string]string{}
		}
		out[file] = column
	}
	return out, twice
}

// numberColumns are the names of cols that are Number columns.
func numberColumns(cols []lab.Column) []string {
	names := []string{}
	for _, c := range cols {
		if c.Type == lab.Number {
			names = append(names, c.Name)
		}
	}
	return names
}

// findMonthColumns are the month columns of the Dashboard's Dataset, found by reading its current Parts from the store
// one at a time, bar the Part except (an ID, 0 for none) whose values are going away; empty when it has none. A store
// that cannot give a Part is ErrStore.
func (d *Dataset) findMonthColumns(ctx context.Context, dashboardID int64, cols []lab.Column, except int64) ([]string, error) {
	parts, _, err := d.Keys(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	scan := lab.NewMonthScan(cols)
	for _, p := range parts {
		if p.Rows == 0 || p.ID == except {
			continue
		}
		data, err := d.store.Get(ctx, p.Key)
		if err != nil {
			return nil, fmt.Errorf("%w: get Part %s: %v", ErrStore, p.Key, err)
		}
		part, err := lab.PartDataset(data, cols)
		if err != nil {
			return nil, fmt.Errorf("read Part %s: %w", p.Key, err)
		}
		scan.Add(part)
	}
	return scan.Columns(), nil
}

// NewKey is a new key in the store: a new one every upload, so a cached copy is never stale.
func NewKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "dataset-" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}
