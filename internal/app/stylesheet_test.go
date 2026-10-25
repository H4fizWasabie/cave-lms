package app_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The stylesheet keeps to the design system in DESIGN.md: every colour, font size, spacing step and radius is a token
// defined in :root, and rules use the tokens, so the site stays one product as pages are added.

// stylesheet returns the served stylesheet without comments, and its rules with the :root and @font-face blocks taken out.
// The rules also leave out the "Lab: as on main" section: the Lab is out of the refresh and keeps main's values.
func stylesheet(t *testing.T) (all, rules string) {
	t.Helper()
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/static/cavelms.css", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /static/cavelms.css = %d", recorder.Code)
	}
	comments := regexp.MustCompile(`(?s)/\*.*?\*/`)
	all = comments.ReplaceAllString(recorder.Body.String(), "")
	lab := regexp.MustCompile(`(?s)/\* === Lab: as on main .*?/\* === end Lab: as on main === \*/`)
	if !lab.MatchString(recorder.Body.String()) {
		t.Fatal(`the stylesheet has no "Lab: as on main" section; if the Lab was redesigned, drop its exemption here`)
	}
	rules = comments.ReplaceAllString(lab.ReplaceAllString(recorder.Body.String(), ""), "")
	rules = regexp.MustCompile(`(?s)(:root|@font-face)\s*\{[^}]*\}`).ReplaceAllString(rules, "")
	return all, rules
}

// tokens returns the custom properties :root defines with the given prefix.
func tokens(all, prefix string) []string {
	var names []string
	for _, block := range regexp.MustCompile(`(?s):root\s*\{([^}]*)\}`).FindAllStringSubmatch(all, -1) {
		for _, m := range regexp.MustCompile(`(--` + prefix + `[a-z0-9-]*)\s*:`).FindAllStringSubmatch(block[1], -1) {
			names = append(names, m[1])
		}
	}
	return names
}

// declarations returns the values of the given properties in the rules.
func declarations(rules string, property string) []string {
	var values []string
	for _, m := range regexp.MustCompile(`(?:^|[;{\s])(` + property + `)\s*:\s*([^;{}]+)`).FindAllStringSubmatch(rules, -1) {
		values = append(values, strings.TrimSpace(m[2]))
	}
	return values
}

func TestTheStylesheetsColoursAreAllTokensDefinedAtTheTop(t *testing.T) {
	all, rules := stylesheet(t)
	if got := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(`).FindAllString(rules, -1); len(got) > 0 {
		t.Errorf("rules hard-code %d colours (%v); use the colour tokens in :root", len(got), got)
	}
	var hexes []string
	for _, block := range regexp.MustCompile(`(?s):root\s*\{([^}]*)\}`).FindAllStringSubmatch(all, -1) {
		hexes = append(hexes, regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`).FindAllString(block[1], -1)...)
	}
	if len(hexes) > 11 {
		t.Errorf(":root defines %d colours (%v), want at most 11", len(hexes), hexes)
	}
}

func TestTheStylesheetUsesEightFontSizesAndTheFontVariables(t *testing.T) {
	all, rules := stylesheet(t)
	if got := tokens(all, "fs-"); len(got) != 8 {
		t.Errorf("type scale has %d sizes (%v), want 8", len(got), got)
	}
	for _, v := range declarations(rules, "font-size") {
		if !regexp.MustCompile(`^var\(--fs-[a-z0-9-]+\)$|^inherit$`).MatchString(v) {
			t.Errorf("font-size: %s is not a type-scale token", v)
		}
	}
	for _, v := range append(declarations(rules, "font-family"), declarations(rules, "font")...) {
		if regexp.MustCompile(`[0-9.]+(rem|em|px|%)|["']|\b(serif|sans-serif|monospace|ui-monospace)\b`).MatchString(regexp.MustCompile(`var\([^)]*\)`).ReplaceAllString(v, "")) {
			t.Errorf("font declaration %q names a size or family; use the --fs- and --font- variables", v)
		}
	}
}

func TestTheStylesheetSpacesWithEightSteps(t *testing.T) {
	all, rules := stylesheet(t)
	if got := tokens(all, "space-"); len(got) != 8 {
		t.Errorf("spacing scale has %d steps (%v), want 8", len(got), got)
	}
	allowed := regexp.MustCompile(`^(0|auto|-?[12]px|var\(--(space-[1-8]|gutter)\)|calc\(.*\))$`)
	for _, property := range []string{"padding(?:-[a-z-]+)?", "margin(?:-[a-z-]+)?", "gap", "row-gap", "column-gap"} {
		for _, v := range declarations(rules, property) {
			for _, part := range splitValue(strings.TrimSuffix(v, " !important")) {
				if !allowed.MatchString(part) || (strings.HasPrefix(part, "calc(") && regexp.MustCompile(`[0-9.]+(rem|em|px)`).MatchString(part)) {
					t.Errorf("%s: %s uses %q, not a spacing step", property, v, part)
				}
			}
		}
	}
}

func TestTheStylesheetRoundsCornersWithTheRadiusTokens(t *testing.T) {
	_, rules := stylesheet(t)
	for _, v := range declarations(rules, "border(?:-[a-z]+)*-radius") {
		for _, part := range splitValue(v) {
			if !regexp.MustCompile(`^(0|50%|var\(--radius(-[a-z]+)?\))$`).MatchString(part) {
				t.Errorf("border-radius: %s uses %q, not a radius token", v, part)
			}
		}
	}
}

// Windows High Contrast drops box-shadows, so a text field that swaps its outline for a shadow on focus shows no ring.
func TestTextFieldsKeepAFocusOutlineInForcedColours(t *testing.T) {
	_, rules := stylesheet(t)
	for _, m := range regexp.MustCompile(`([^{}]*:focus[^{}]*)\{([^}]*)\}`).FindAllStringSubmatch(rules, -1) {
		if regexp.MustCompile(`outline\s*:\s*(none|0)\b`).MatchString(m[2]) {
			t.Errorf("%s{%s} removes the focus outline; keep a transparent one so forced colours can draw it", strings.TrimSpace(m[1]), m[2])
		}
	}
}

func TestTheStylesheetHasThreeBreakpoints(t *testing.T) {
	all, _ := stylesheet(t)
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\((?:max|min)-width:\s*([^)]+)\)`).FindAllStringSubmatch(all, -1) {
		seen[m[1]] = true
	}
	if len(seen) > 3 {
		t.Errorf("stylesheet breaks at %d widths (%v), want at most 3", len(seen), seen)
	}
}

// splitValue splits a CSS value on spaces outside parentheses.
func splitValue(v string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range v {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ' ' && depth == 0:
			if i > start {
				parts = append(parts, v[start:i])
			}
			start = i + 1
		}
	}
	if start < len(v) {
		parts = append(parts, v[start:])
	}
	return parts
}
