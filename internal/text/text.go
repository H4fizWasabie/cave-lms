// Package text is the catalog of every text Cave writes, in English and Malay, by a stable key.
// A text with values to fill in uses fmt verbs; a translation that reorders them uses %[2]s and the like.
// A text that depends on a count is two keys, key.one and key.other.
// Content Admins type is never in here: it is shown as written.
package text

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Languages Cave speaks; the first is the default.
var Languages = []string{"ms", "en"}

// Supported reports whether lang is one of Languages.
func Supported(lang string) bool { return slices.Contains(Languages, lang) }

type entry struct{ en, ms string }

// T returns the text for key in lang, with args filled in. A missing key comes back as itself.
func T(lang, key string, args ...any) string {
	e, ok := catalog[key]
	if !ok {
		return key
	}
	s := e.ms
	if lang == "en" {
		s = e.en
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// N returns key.one when n is 1, else key.other, with n filled in first and then args.
func N(lang, key string, n int, args ...any) string {
	form := ".other"
	if n == 1 {
		form = ".one"
	}
	return T(lang, key+form, append([]any{n}, args...)...)
}

// Has reports whether the catalog has key, or key.one and key.other.
func Has(key string) bool {
	_, ok := catalog[key]
	_, plural := catalog[key+".other"]
	return ok || plural
}

var months = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"ms": {"Januari", "Februari", "Mac", "April", "Mei", "Jun", "Julai", "Ogos", "September", "Oktober", "November", "Disember"},
}

// Date writes day as "22 Oktober 2026" or "22 October 2026".
func Date(lang string, day time.Time) string {
	names, ok := months[lang]
	if !ok {
		names = months[Languages[0]]
	}
	return fmt.Sprintf("%d %s %d", day.Day(), names[day.Month()-1], day.Year())
}

// FromAcceptLanguage returns the first of Languages an Accept-Language header names, else the default.
// ponytail: q-values are ignored; browsers list languages in the order they prefer.
func FromAcceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		tag, _, _ := strings.Cut(part, ";")
		primary, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
		if lang := strings.ToLower(primary); Supported(lang) {
			return lang
		}
	}
	return Languages[0]
}

var verb = regexp.MustCompile(`%(\[\d+\])?[a-z]`)

// CheckCatalog reports every key missing a language, filling in different values in its two languages,
// or with only one of its singular and plural forms.
func CheckCatalog() error {
	var problems []string
	for key, e := range catalog {
		if e.en == "" || e.ms == "" {
			problems = append(problems, key+" lacks a language")
		}
		if !slices.Equal(verbs(e.en), verbs(e.ms)) {
			problems = append(problems, key+" fills in different values in English and Malay")
		}
		if base, ok := strings.CutSuffix(key, ".one"); ok {
			if _, ok := catalog[base+".other"]; !ok {
				problems = append(problems, key+" has no .other")
			}
		}
		if base, ok := strings.CutSuffix(key, ".other"); ok {
			if _, ok := catalog[base+".one"]; !ok {
				problems = append(problems, key+" has no .one")
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("text catalog:\n%s", strings.Join(problems, "\n"))
}

// verbs lists the values a text fills in, in argument order, so a reordered translation still matches.
func verbs(s string) []string {
	found := verb.FindAllString(s, -1)
	next := 1
	for i, v := range found {
		if !strings.HasPrefix(v, "%[") {
			found[i] = fmt.Sprintf("%%[%d]%s", next, v[1:])
			next++
		}
	}
	sort.Strings(found)
	return found
}
