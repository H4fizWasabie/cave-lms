package lab_test

import (
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

func TestThemeByKeyFindsEachThemeByItsKeyAndNoOtherKey(t *testing.T) {
	for _, want := range lab.Themes {
		if got, ok := lab.ThemeByKey(want.Key); !ok || got != want {
			t.Errorf("ThemeByKey(%q) = %v, %v; want %v, true", want.Key, got, ok, want)
		}
	}
	for _, key := range []string{"neon", "", "Corporate"} {
		if got, ok := lab.ThemeByKey(key); ok || got != (lab.Theme{}) {
			t.Errorf("ThemeByKey(%q) = %v, %v; want the zero Theme and false", key, got, ok)
		}
	}
}
