package app

// PROTOTYPE (#504, throwaway): four layouts of the redesigned Classroom Folder, on one dev-only route with stub data.
//
//	GET /prototype/my-folder?variant=A|B|C|D[&empty=1]  (D, the default, is B's layout with C's Dashboard cards)
//
// Not registered when a.production is true. No auth, no database, no persistence: every form posts back to this page.
// Delete this file, templates/proto_my_folder.html and static/proto-504.{css,js} with the prototype branch.

import (
	"net/http"
	"time"
)

type protoUse struct {
	ID   int64
	Name string
}

type protoWorkbook struct {
	ID       int64
	Name     string
	Size     int64
	Uploaded string
	UsedOn   []protoUse
}

type protoPage struct {
	Current  bool
	Size     int64
	Uploaded string
}

type protoDashboard struct {
	ID        int64
	Name      string
	Live      string // the Deployment's address, "" when never deployed
	Pages     []protoPage
	Workbooks []protoUse // the Workbooks it used (ID and Name are the Workbook's)
}

type protoVariant struct {
	Key, Name string
	Current   bool
}

type protoFolderPage struct {
	pageBase
	Variant    string
	Variants   []protoVariant
	Prev, Next string // addresses of the neighbouring variants, empty flag kept
	EmptyURL   string // toggles the empty state
	Empty      bool
	Workbooks  []protoWorkbook
	Unused     []protoWorkbook
	Dashboards []protoDashboard
	Courses    []folderCourse
	UsedMB     int
	CapMB      int
	// The Storage breakdown for variant C, in MB.
	WorkbooksMB, PagesMB, PartsMB int
}

func (a *App) prototypeMyFolder(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	at := func(day, hour, min int) string {
		return dateTimeIn(lang, time.Date(2026, 10, day, hour, min, 0, 0, malaysia))
	}
	const user = "aisyah"
	live := func(slug string) string { return a.pagesBaseURL + "/" + user + "/" + slug + "/" }
	sales, regional, board := protoUse{1, "Sales Overview"}, protoUse{2, "Regional Targets"}, protoUse{3, "Board Pack Q3"}

	page := &protoFolderPage{
		pageBase: pageBase{Learner: &learner{Name: "Aisyah Rahman", Username: user}},
		Variant:  r.URL.Query().Get("variant"),
		Empty:    r.URL.Query().Has("empty"),
		CapMB:    1024,
	}
	if v := page.Variant; v != "A" && v != "B" && v != "C" {
		page.Variant = "D"
	}
	keep := ""
	if page.Empty {
		keep = "&empty=1"
		page.EmptyURL = "?variant=" + page.Variant
	} else {
		page.EmptyURL = "?variant=" + page.Variant + "&empty=1"
	}
	names := map[string]string{"A": "Stacked", "B": "Side index", "C": "Dashboard-first", "D": "Index + cards"}
	order := []string{"A", "B", "C", "D"}
	for i, k := range order {
		page.Variants = append(page.Variants, protoVariant{k, names[k], k == page.Variant})
		if k == page.Variant {
			page.Prev = "?variant=" + order[(i+len(order)-1)%len(order)] + keep
			page.Next = "?variant=" + order[(i+1)%len(order)] + keep
		}
	}

	if !page.Empty {
		page.UsedMB, page.WorkbooksMB, page.PagesMB, page.PartsMB = 640, 410, 140, 90
		page.Workbooks = []protoWorkbook{
			{1, "Sales by Region 2026.xlsx", 2202009, at(24, 14, 20), []protoUse{sales, regional, board}},
			{2, "Monthly_Operations_Report_Kuala_Lumpur_Penang_Johor_Bahru_Combined_Final_v7_approved.xlsx", 5872025, at(21, 9, 5), []protoUse{board}},
			{3, "Customer Feedback Survey.xlsx", 318464, at(17, 16, 42), []protoUse{regional}},
			{4, "Training Budget Draft.xlsx", 96256, at(12, 11, 0), nil},
			{5, "Staff Roster.xlsx", 148480, at(8, 20, 31), nil},
			{6, "Inventory Count Oct.xlsx", 757760, at(2, 8, 15), nil},
		}
		for _, wb := range page.Workbooks {
			if len(wb.UsedOn) == 0 {
				page.Unused = append(page.Unused, wb)
			}
		}
		page.Dashboards = []protoDashboard{
			{Name: sales.Name, ID: 1, Live: live("sales-overview"), Pages: []protoPage{
				{true, 1887436, at(25, 10, 12)}, {false, 1782579, at(22, 17, 48)}, {false, 1677721, at(19, 13, 3)}, {false, 1572864, at(14, 9, 30)}}},
			{Name: regional.Name, ID: 2, Pages: []protoPage{{true, 912261, at(26, 15, 40)}}},
			{Name: board.Name, ID: 3, Live: "", Pages: []protoPage{{true, 3250585, at(27, 11, 22)}, {false, 3040870, at(23, 14, 9)}}},
		}
		for i, d := range page.Dashboards {
			for _, wb := range page.Workbooks {
				for _, u := range wb.UsedOn {
					if u.ID == d.ID {
						page.Dashboards[i].Workbooks = append(page.Dashboards[i].Workbooks, protoUse{wb.ID, wb.Name})
					}
				}
			}
		}
		file := func(id int64, name string, size int64) submissionFile {
			return submissionFile{ID: id, Name: name, Size: size}
		}
		page.Courses = []folderCourse{
			{ID: 1, Title: "Excel for Public Sector Analysts", Enrolled: true, Assignments: []folderAssignment{
				{ID: 1, Title: "Assignment 1: Clean a payroll sheet", Published: true, Comment: "Good use of lookups. Next time, name your sheets.",
					Files: []submissionFile{file(1, "payroll_clean_aisyah.xlsx", 245760)}},
				{ID: 2, Title: "Assignment 2: Build a pivot summary", Published: true,
					Files: []submissionFile{file(2, "pivot_summary.xlsx", 389120), file(3, "notes.pdf", 51200)}},
			}},
			{ID: 2, Title: "Introduction to Data Dashboards", Enrolled: false, Assignments: []folderAssignment{
				{ID: 3, Title: "Final project", Published: true, Comment: "Clear layout. Well done.",
					Files: []submissionFile{file(4, "final_project_dashboard.zip", 4404019)}},
			}},
		}
	}
	a.render(w, r, http.StatusOK, "proto_my_folder", page)
}

// prototypeBack answers any prototype form: nothing happens, you land back on the page.
func prototypeBack(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.URL.RequestURI(), http.StatusSeeOther)
}
