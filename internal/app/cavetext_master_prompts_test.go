package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func masterPromptForm(imageName, imageShortcut, imageBody, dashName, dashShortcut, dashBody string) url.Values {
	return url.Values{
		"image_name": {imageName}, "image_shortcut": {imageShortcut}, "image_body": {imageBody},
		"dashboard_name": {dashName}, "dashboard_shortcut": {dashShortcut}, "dashboard_body": {dashBody},
	}
}

func (s *site) saveMasterPrompts(form url.Values) (*http.Response, string) {
	s.t.Helper()
	return s.post("/admin/cavetext/master-prompts", form)
}

// describePrompts shows what me sent of the Image and Dashboard master prompts.
func describePrompts(me caveTextMe) string {
	out := ""
	for _, p := range me.MasterPrompts {
		out += "[" + p.Kind + " " + p.Name
		if p.Shortcut != nil {
			out += " " + *p.Shortcut
		}
		if p.Body != nil {
			out += fmt.Sprintf(" body=%d", len(*p.Body))
		}
		out += "]"
	}
	return out
}

func TestTheImageAndDashboardMasterPromptsGoInFullToProAndByNameToFreeLearners(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")

	res, body := ada.caveText(http.MethodGet, "me", token)
	expectStatus(t, res, body, http.StatusOK, `"masterPrompts":[{"kind":"image","name":"Image prompt"},{"kind":"dashboard","name":"Dashboard prompt"}]`)
	for _, leak := range []string{"/image", "/dashboard", "{form"} {
		if strings.Contains(body, leak) {
			t.Fatalf("a free Learner's me has %q: %s", leak, body)
		}
	}

	s.givePro("ada@example.com")
	me := ada.me(token)
	if got := describePrompts(me); !strings.HasPrefix(got, "[image Image prompt /image body=") || !strings.Contains(got, "][dashboard Dashboard prompt /dashboard body=") {
		t.Fatalf("Pro: master prompts = %s, want both in full", got)
	}
	for _, p := range me.MasterPrompts {
		if !strings.Contains(*p.Body, "{formtext") && !strings.Contains(*p.Body, "{formmenu") {
			t.Errorf("default %s body has no form fields: %q", p.Kind, *p.Body)
		}
	}

	s.agePro(400)
	if got := describePrompts(ada.me(token)); got != "[image Image prompt][dashboard Dashboard prompt]" {
		t.Fatalf("lapsed: master prompts = %s, want names only", got)
	}
}

func TestNoSnippetTakesOrClashesWithAMasterPromptShortcut(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	mine := ada.createSnippet(token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})

	for _, pro := range []bool{false, true} {
		if pro {
			s.givePro("ada@example.com")
		}
		for _, c := range []struct{ shortcut, message string }{
			{"/image", `\"Image prompt\" already uses /image.`},
			{"/IMAGE", `\"Image prompt\" already uses /image.`},
			{"/im", `/im would always fire before /image (\"Image prompt\") finished typing.`},
			{"/images", `/image (\"Image prompt\") would always fire before /images finished typing.`},
			{"/dashboard", `\"Dashboard prompt\" already uses /dashboard.`},
		} {
			res, body := ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "New", Shortcut: c.shortcut, Body: "y"})
			expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
			res, body = ada.caveTextSend(http.MethodPut, fmt.Sprintf("snippets/%d", mine.ID), token, snippet{Folder: "Work", Name: "Review", Shortcut: c.shortcut, Body: "x"})
			expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
		}
		// An import with one clashing Snippet writes none.
		res, body := ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": []snippet{
			{Folder: "Imported", Name: "Fine", Shortcut: "/fine", Body: "1"},
			{Folder: "Imported", Name: "Mine", Shortcut: "/dashboard", Body: "2"},
		}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, `\"Dashboard prompt\" already uses /dashboard.`)
		if got, want := describe(ada.snippets(token)), `[Work/Review /review "x" master=true]`; got != want {
			t.Fatalf("pro=%v: Snippets = %s, want %s", pro, got, want)
		}
	}
}

func TestAnyAdminEditsTheImageAndDashboardMasterPrompts(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	res, body := s.get("/admin/cavetext/settings")
	for _, want := range []string{`name="image_name" value="Image prompt"`, `name="image_shortcut" value="/image"`, `name="image_body"`,
		`name="dashboard_name" value="Dashboard prompt"`, `name="dashboard_shortcut" value="/dashboard"`, `name="dashboard_body"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	before := describePrompts(ada.me(token))

	for _, c := range []struct {
		form    url.Values
		message string
	}{
		{masterPromptForm("Image", "/my image", "x", "Dash", "/dash", "y"), "Shortcuts cannot contain spaces."},
		{masterPromptForm("Image", "/img", "x", "Dash", "/da\tsh", "y"), "Shortcuts cannot contain spaces."},
		{masterPromptForm("Image", "", "x", "Dash", "/dash", "y"), "A shortcut is needed."},
		{masterPromptForm("Image", "/img", "x", "Dash", "/IMG", "y"), "already uses /img."},
		{masterPromptForm("Image", "/img", "x", "Dash", "/img2", "y"), "would always fire before"},
		{masterPromptForm(" ", "/img", "x", "Dash", "/dash", "y"), "name"},
		{masterPromptForm("Image", "/img", "x", "Dash", "/dash", "  \n"), "Dashboard master prompt"},
		{masterPromptForm("Image", "/img", strings.Repeat("x", 20001), "Dash", "/dash", "y"), "20,000 characters"},
	} {
		res, body = s.saveMasterPrompts(c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
		// The posted values stay in the form to fix.
		expectStatus(t, res, body, http.StatusUnprocessableEntity, `name="dashboard_name" value="Dash"`)
	}
	if got := describePrompts(ada.me(token)); got != before {
		t.Fatalf("after refused saves: %s, want %s", got, before)
	}

	res, body = s.saveMasterPrompts(masterPromptForm("Picture", "/pic", "Draw {formtext: name=subject}.", "Charts", "/charts", "Chart {formtext: name=data}."))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Master prompts saved.")
	expectStatus(t, res, body, http.StatusOK, `name="image_shortcut" value="/pic"`)
	expectStatus(t, res, body, http.StatusOK, "Draw {formtext: name=subject}.")
	if strings.Contains(body, "clashes") {
		t.Fatalf("no Snippet clashes, yet the page warns: %s", body)
	}
	me := ada.me(token)
	if got, want := describePrompts(me), "[image Picture /pic body=30][dashboard Charts /charts body=28]"; got != want {
		t.Fatalf("after the save: %s, want %s", got, want)
	}
	// The old shortcut is free again; the new one is reserved.
	ada.createSnippet(token, snippet{Folder: "Work", Name: "Mine", Shortcut: "/image", Body: "x"})
	res, body = ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "Pic", Shortcut: "/pic", Body: "x"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `\"Picture\" already uses /pic.`)

	// The question form leaves the master prompts as they are.
	res, body = s.saveCaveTextQuestion("What do you know about me?")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := describePrompts(ada.me(token)); got != "[image Picture /pic body=30][dashboard Charts /charts body=28]" {
		t.Fatalf("after saving the question: %s", got)
	}
}

func TestAMasterPromptShortcutSomeSnippetsClashWithIsSavedAndTheAdminSeesHowManyLearners(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bob, bobToken := s.newCaveTextLearner("Bob Byte", "bob@example.com")
	cy, cyToken := s.newCaveTextLearner("Cy Code", "cy@example.com")
	ada.createSnippet(adaToken, snippet{Folder: "Work", Name: "Pic", Shortcut: "/pic", Body: "x"})
	ada.createSnippet(adaToken, snippet{Folder: "Work", Name: "Chart", Shortcut: "/chart", Body: "x"})
	bob.createSnippet(bobToken, snippet{Folder: "Work", Name: "P", Shortcut: "/PI", Body: "x"})
	cy.createSnippet(cyToken, snippet{Folder: "Work", Name: "Other", Shortcut: "/other", Body: "x"})

	// Ada's two Snippets clash, one with each: she counts once.
	res, body := s.saveMasterPrompts(masterPromptForm("Picture", "/pic", "Draw it.", "Charts", "/chart", "Chart it."))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Master prompts saved.")
	expectStatus(t, res, body, http.StatusOK, "2 Learners have a Snippet whose shortcut clashes")

	// The clashing Snippets stay; editing one asks for another shortcut.
	if n := len(ada.snippets(adaToken)); n != 2 {
		t.Fatalf("Ada has %d Snippets, want 2 kept", n)
	}
	res, body = s.saveMasterPrompts(masterPromptForm("Picture", "/pic", "Draw it.", "Dashboard prompt", "/other", "Chart it."))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "3 Learners have a Snippet whose shortcut clashes")
	res, body = s.saveMasterPrompts(masterPromptForm("Image prompt", "/image", "Draw it.", "Dashboard prompt", "/dashboard", "Chart it."))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	if strings.Contains(body, "clashes") {
		t.Fatalf("no clash left, yet the page warns: %s", body)
	}
}

func TestOneLearnerWithAClashingSnippetIsCountedAsOne(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.createSnippet(token, snippet{Folder: "Work", Name: "Pic", Shortcut: "/pic", Body: "x"})
	res, body := s.saveMasterPrompts(masterPromptForm("Picture", "/pic", "Draw it.", "Dashboard prompt", "/dashboard", "Chart it."))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "1 Learner has a Snippet whose shortcut clashes")
}
