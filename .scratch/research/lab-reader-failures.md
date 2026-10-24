# Where today's Lab reader fails on real spreadsheets

Research for [#207](https://github.com/H4fizWasabie/cave-lms/issues/207), part of the map [#205](https://github.com/H4fizWasabie/cave-lms/issues/205). Question: run today's `lab.Read` on each collected spreadsheet with the Template that fits best, and list every way it fails or misreads, grouped by kind and counted.

## Method

- Data: the 8 anonymised `.xlsx` files from #206 (one vet clinic; kept privately in `~/cave-lab-samples`, never in the repo). 65 sheets in all. This note describes shapes and column headers only, no cell values.
- Code under test: `lab.Read`, `Template.Guess` and `lab.Generate` at `origin/main` (d492982): `internal/lab/lab.go`, `templates.go`, `generate.go`.
- Harness: `internal/lab/research_realfiles_test.go` on this branch, skipped unless `LAB_SAMPLES` is set:
  `LAB_SAMPLES=~/cave-lab-samples go test ./internal/lab/ -run TestResearchRealFiles -v` (about 5 minutes).
  For each file it runs `Read` with all four Templates (Finance, Inventory, HR, General) on the file as uploaded, then on every sheet alone (the others deleted, so it is the first sheet). It runs three passes per sheet: as is; with the rows above a guessed header removed; and also with the columns that have a blank header cell removed. For sheets that read, it runs `Guess` and `Generate`, and it prints the blocks General would draw. `TestResearchShapes` and `TestResearchColumnShapes` print layouts as masks (letters as `a`, digits as `9`), so the layouts can be checked without their contents.
- Error kinds below are `Read`'s own catalog keys (`lab.err.*`). "Misread" means `Read` succeeded but the Dataset or the guessed columns are wrong.

## The files

| # | File (short name) | Sheets | Shape |
|---|---|---|---|
| D | Daily Report (Management) | 32 | one sheet per day (30), plus 2 August sheets (cash closing, refunds) |
| S | Service & Product Sales (Jun-Aug) | 7 | a Summary sheet, then 6 month x service/product sheets of 3.9k-6.4k rows |
| H | High movement items Jan-Aug | 7 | the full item list, then 6 copies filtered by price band |
| R | Roster Oct | 8 | one sheet per department, staff x day grid of shift codes |
| T | Test & Parameters | 8 | unrelated tables: test notes, item names, price list, product list, cost breakdowns, supplier list |
| SB | Stock Balance History | 1 | system export, 1,142 items |
| SD | Stock Detail | 1 | system export, 949 dated stock movements |
| SM | Stock Movement | 1 | system export, 1,132 items with opening, in, out and closing |

## What happens on upload today (first sheet only)

| File | Finance | Inventory | HR | General | What the Learner sees |
|---|---|---|---|---|---|
| D | fail | fail | fail | fail | row 1 is blank, so `Read` finds zero columns and reports **every filled cell as a stray cell (about 500-800 errors)** plus missing columns |
| S | fail | fail | fail | fail | the first sheet is the Summary: missing columns, stray cells, blank required cells |
| H | fail (no date) | reads, misguessed | reads, nonsense | reads, poor | see "Files that read" |
| R | fail | fail | fail | fail | 6 unnamed columns (B-G), from title rows above the grid |
| T | fail | fail | fail | fail | 2 unnamed columns (B, E), from a merged header cell and an index column |
| SB | fail (no date) | reads, misguessed | reads, nonsense | reads, poor | see below |
| SD | fail (`not-date` x949 on Date) | reads, misguessed | reads, nonsense | reads, poor | see below |
| SM | fail (no date) | reads, misguessed | reads, nonsense | reads, poor | see below |

4 of 8 files read with at least one Template. **None reads into a correct Dashboard**: every Template that reads guesses a wrong column for at least one required blank (details below). Finance reads no sheet of any file, because no real file has a column `Read` types as Date.

Per sheet: 15 of 65 sheets read as is with some Template (H 7, S's 3 service sheets, SB, SD, SM, T's Sheet5 and Sheet9). With title rows and unnamed columns removed by hand, 41 read (adding 24 of the 30 daily sheets and both August sheets of D). The other 24 still fail.

## Failure kinds, counted

"Files / sheets" counts where the kind occurs. "Read today" says what `Read` does with it.

| Kind | Files | Sheets | Where | Read today |
|---|---|---|---|---|
| Title rows above the header (header not in row 1) | 3 (D, R, S) | 41 | D header on row 8 (daily) or 4 (August); R grid header on rows 4-7; S Summary | fails: `unnamed-column`, or `stray-cell` floods when row 1 is blank (D, 30 sheets) |
| Header over two or more rows | 2 (D, R) | 38 | D: group row above (merged "Payment"-style bands) and a `(+)/(-)` sub-header row below; R: weekday-name row over a day-number row | fails, or, once trimmed, the sub-header row is read as a data row |
| Merged header cells | 2 (D, T) | 34 | D 31 (group bands, titles); T 3 (a header merged over 2-17 columns, so all but the first are blank) | fails: `unnamed-column` for each covered column |
| Merged cells in the data (grouped rows) | 1 (T) | 2 | T Main (104 merges), Detail Cost (133) | blank cells under each group label |
| Unnamed columns (blank header cell) | 3 (D, R, T) | 45 | D: blank lead column A and spacer column J; R: row-number and staff-name columns have no header; T: index columns and merged headers | fails: `unnamed-column`. The most common hard stop after title rows |
| Fully empty columns | 3 (D, H, T) | 42 | D spacer columns (32); H "average for 8 months" empty in 6 of 7 sheets; T 4 sheets (some blanked by the anonymising, e.g. bank details) | reads as an empty Text column; General ignores it |
| Several sheets | 5 (D, H, R, S, T) | 62 | see "Several sheets" below | only the first sheet is read; the rest are silently ignored |
| Wrong first sheet (first unreadable, a later one readable) | 2 (S, T) | - | S: Summary first, 6 data sheets after; T: notes sheet first, list sheets 3 and 8 readable | whole upload fails |
| Total, summary or footer rows inside the data | 2 (D, S) | 36 | S: a label-only total row at the end of all 6 month sheets; D: a block of totals and notes under every daily table, labels sitting in the amount columns | S: `empty-cell` on required blanks, or a junk row in General; D: amount columns become Text |
| Header-less summary sheet (labels beside values, blocks stacked) | 1 (S) | 1 | S Summary: 3 monthly blocks, label column then value column | fails |
| Extra cells or tables beside or below the table | 4 (D, R, S, T) | 45 | R: per-staff count columns to the right after a blank column, and a legend list below (8); D: notes right of the last header (6) and footer blocks (30, counted above); T: notes past the last header (6); S Summary | fails: `stray-cell` |
| Side-by-side tables proper | 1 (R) | 8 | R's count columns form a second table beside the grid. (D's left and right halves, invoices and payments, share rows: one record split by a spacer column, not two tables) | fails: `stray-cell` or `unnamed-column` |
| Wide layout, days across columns | 1 (R) | 8 | R: 28-31 day columns of shift codes per staff row | fails: `same-name` x24 (weekday names repeat) even when trimmed; would need unpivoting, which `Read` cannot do |
| Date-times as text with AM/PM (`dd/mm/yyyy hh:mm:ss AM`) | 2 (S, SD) | 7 | S "Created Date & Time" (about 30,000 cells), SD "Date" (949) | misread: column typed Text, so no line chart and no Finance; with Finance, `not-date` per cell |
| Other text dates `Read` cannot parse | 2 (SD, D) | 2 | SD "Expiry Date" as ISO `yyyy-mm-dd`; D Refund dates as `dd.mm.yy` (two-digit year) | typed Text |
| No date column (date only in sheet name or title) | 1 (D) | 30 | each daily sheet is one day; the day exists only in the sheet name | nothing to chart by month, even if combined |
| Date split over columns (day number, month name) | 1 (D) | 1 | D Cash Closing "Date" holds day-of-month numbers, "Month" the month | typed Text (one `-` cell); as a Finance date the numbers would read as Excel serials (1900-01-15 for 15) |
| Times as text (`9.30am`) | 1 (D) | 30 | D "Time" | Text; harmless today |
| `-` for zero or none in number columns | 3 (SB, SD, SM) | 3 | SB "Actual Stock" (387 cells); SM all 8 quantity columns (3,611 cells); SD "Stock In." and "Stock Out." | misread: the column becomes Text, so it cannot be chosen for a Number blank (`wrong-type`) and Guess takes another Number column |
| A stray label in a number column | 2 (D, T) | 31 | D: footer labels in the amount columns of all 30 daily sheets; T Sheet5: one text "Price" | column becomes Text (General loses the card; Inventory `not-number`) |
| Numbers stored as text | 2 (SB, SM) | 2 | 3,197 and 4,684 cells | **reads fine**: `number()` parses numeric text |
| Grouped rows (label on the first row only, blank below) | 1 (T) | 3 | T Test & Parameters (60 of 65 rows), Main (157 of 165), Detail Cost (243 of 308) | blank cells; `empty-cell` on any required blank |
| Header-like rows repeated inside the data | 2 (D, T) | 31 | D's sub-header row (30); T Detail Cost | read as data rows |
| Blank cells in a column a required blank uses | 2 (S, T) | 7 | S total rows (6); T Sheet4 "Cost Price" blank on 62 rows | fails: `empty-cell` |
| Over MaxRows (5,000) | 1 (S) | 3 | S product sheets: 5,551, 5,778, 6,381 rows | fails: `too-many-rows` |
| Duplicate header names | 1 (R) | 8 | weekday names | fails: `same-name` |

Not seen in this set: Malay headers, mixed day/month order (every slash date is day first: 2,399-3,939 cells per sheet have a first part over 12, none a second part), RM-prefixed amounts, `.xls`/CSV.

### Several sheets

- Same-shape sheets that belong together: D (30 days), S (3 months x service/product), R (8 departments). 3 files.
- Filtered copies of one table: H (6 price bands of sheet 1). Sheet 1 is the right one.
- Unrelated tables in one workbook: T (8), D (the 2 August sheets). 2 files.
- Combined sizes: D's 30 days about 2,800 rows (under MaxRows); S's 6 sheets 30,245 rows (service 12,535, product 17,710), far over.

## Files that read: is the guessing right, and what would the Dashboard show?

No file reads into a Dashboard that answers its Template's question without the Learner rechoosing columns, and for three files the right column cannot be chosen at all because `-` made it Text.

- **SB Stock Balance, Inventory** (the best fit in the set): item = "Product Name" (right), category = "Category" (right), **quantity = "Cost Price" (wrong)**: "Actual Stock" is Text because of `-`. No price, because "Cost Price"/"Selling Price" do not match the exact guesses ("price", "unit price"). No reorder level. Dashboard: "Total items" sums cost prices; "Stock by category" charts cost price; no stock value, no low stock.
- **SM Stock Movement, Inventory**: item = "SKU Code" (wanted "Product Name"), **quantity = "Opening Value" (money, not a count)**: all quantity columns are Text because of `-`. Dashboard: totals of opening value labelled as items.
- **SD Stock Detail, Inventory**: **item = "Date"** (the date-time is Text, so it is the first Text column), quantity = "Balance" (a running balance; summing it means nothing). It is a dated ledger, not stock on hand.
- **H High movement, Inventory** (all 7 sheets): item = "SKU Code" (wanted "Product Name"), quantity = "movement out" (units moved over 8 months, not stock on hand), category right, price not guessed ("selling price"). Dashboard: "Total items" is units sold, "Stock by category" is sales by category: useful numbers under wrong labels.
- **HR "reads" 13 sheets in 6 files** (H's 7, SB, SD, SM, T Sheet5 and Sheet9, D Cash Closing once trimmed) none of which lists staff: HR's required blanks are Text, and Guess falls back to the first unused Text columns (name = "SKU Code", department = "Product Name", and so on). A successful `Read` is no evidence of a fit.
- **General** on every readable sheet: the bar chart groups by the first Text column, which here is a code, a name or a date-time string, so it has one bar per row (hundreds to thousands). Every Number column becomes a summed card, including unit prices and running balances. No line chart, because no readable sheet has a real Date column (except D Cash Closing once trimmed, where it charts "Date Collection", the wrong date). On S's service sheets the "Price" card is the month's real revenue; it is the only meaningful number General produces in the set.
- **Finance**, by reading the code (not run, since no sheet reads): if S's date-times parsed, Finance would guess type = "Type" (values Service/Product, not Masuk/Keluar), so Balance, Money in and Money out would show RM0 and the plain Total card would be hidden (it has `Unless: "type"`).

## Templates the real files want

| File | Current fit | Wants |
|---|---|---|
| SB | Inventory, once `-` is 0 and guesses accept "actual stock", "cost price" | Inventory as is |
| SM | Inventory, partly (closing balance as quantity) | Stock movement: opening, in, out, closing per item |
| SD | none | Stock movement over time (dated ledger: in/out by month, by type) |
| H | Inventory, wrongly | Sales or best-sellers: units and revenue by item and category |
| S | Finance, wrongly (Type misguess), and only if date-times parse | **Sales**: revenue by month/day, by item, by service vs product, by staff |
| D | none (Finance has no money in/out split) | Daily takings or cash-up: totals by payment method (cash, card, DuitNow), deposits, by doctor |
| R | none | **Roster or shift schedule**: shifts per staff, cover per day, days off; needs wide-to-long unpivot first |
| T | none (General at best for Sheet5/Sheet9) | Price list or catalogue: cost vs selling price, margin by category; the supplier list is a plain table |

No current Template fits 4 of the 8 files at all (D, R, S, T). HR fits none of them. Sales (S, H, D) and Roster (R) are the gaps this set shows most clearly; the client's typical Learners (budgets, claims, asset registers, attendance) are still not in the set (#206).

## What this means for the clean-up decision (#209)

Options, not a decision:

1. **Widen `Read` with deterministic Go rules.** Skip leading blank and title rows (find the header), drop empty unnamed columns, read `-` as blank or 0, parse `dd/mm/yyyy hh:mm[:ss] AM/PM`, ISO and two-digit-year dates, drop label-only total and footer rows, let the Learner pick the sheet. These cover SB, SM, SD, H and S's service sheets, about 5 files. They do not cover R (unpivot), T's grouped rows, D's two-row headers and footers, or the S Summary.
2. **AI clean-up plan, applied by Go and confirmed by the Learner** (the rule already fixed in #205). A JSON plan names the sheet(s), header row(s), columns to keep or rename, rows to drop, value rules (`-` = 0, date format), fill-down for grouped rows, and an unpivot for day-across grids. This is the only option here that reaches R, T and D. It costs a model call per upload, and the plan language must be bounded so Go can check it.
3. **Hybrid.** The rules in option 1 run first; AI is asked only when they leave the sheet unreadable, or to propose the Template and column mapping.
4. **Better errors only; the Learner tidies in Excel.** At minimum, replace the stray-cell flood (D: about 500-800 errors for one upload) with one message ("your headers seem to be on row 8") in Malay and English. This is cheap and needed under any option.
5. **Several sheets** need their own choice, whichever option is taken: pick one sheet, or combine same-shape sheets (D 30, S 6, R 8). Combining S passes 30,000 rows, so MaxRows (5,000) has to rise, or rows have to be aggregated, before combining helps the biggest file.
6. **Guessing** fails even on clean exports: matching is exact-name only, and a required blank falls back to any column of its type, which gives confident nonsense (HR on product lists; Cost Price as quantity). Fuzzy or substring matching, wider guess lists ("actual stock", "cost price", "selling price", "stock alert count"), and leaving a required blank empty when nothing matches would fix more of these sheets than clean-up does. Either AI mapping or plain rules could do it.
7. **New Templates**, Sales and Roster first, then stock movement and price list, decide how much clean-up pays off: for 4 of 8 files a perfect reader still has no Template to feed.

Limits seen, for sizing: largest sheet 6,381 rows x 8 columns; widest 44 columns (R); largest file 1.6 MB; most sheets in a file 32.
