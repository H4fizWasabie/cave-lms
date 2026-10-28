package app_test

import (
	"context"
	"net/url"
	"testing"
)

// Every Lab request reads the Learner's Lab access once, however many handlers and the sidebar ask for it.
func TestALabRequestReadsLabAccessOnce(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	path := ada.createWork("Sales")
	reads := func(do func()) int64 {
		before := ada.application.LabAccessReads()
		do()
		return ada.application.LabAccessReads() - before
	}

	for name, do := range map[string]func(){
		"the Lab":                   func() { ada.get("/lab") },
		"a Dashboard tab":           func() { ada.get(path) },
		"the Deploy tab":            func() { ada.get(path + "/deploy") },
		"a Lab change":              func() { ada.post(path+"/theme", url.Values{}) },
		"the Folder":                func() { ada.get("/my-folder") },
		"My Membership":             func() { ada.get("/my-membership") },
		"a Lab change by a Visitor": func() { admin.second().post(path+"/theme", url.Values{}) },
	} {
		if n := reads(do); n > 1 {
			t.Errorf("%s read Lab access %d times, want once", name, n)
		}
	}
	if n := reads(func() { ada.get(path) }); n != 1 {
		t.Errorf("a Dashboard tab read Lab access %d times, want 1", n)
	}
}

// A request that saves bytes and then shows the storage used shows the figure after the save.
func TestLabAccessReadAfterASaveShowsTheNewStorageUsed(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	var id int64
	if err := ada.db.QueryRow(`SELECT id FROM learners WHERE email = 'ada@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	before, after, err := ada.application.UsedAcrossSave(context.Background(), id, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if after-before != 1234 {
		t.Errorf("storage used went from %d to %d in one request, want 1234 more", before, after)
	}
}
