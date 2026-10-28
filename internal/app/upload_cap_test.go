package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// waitedLine is how the System log says an upload waited for its turn to be read.
const waitedLine = "for its turn to be read"

// Not parallel: the fifth upload must reach Clean-up within 300 ms, which a busy machine does not promise.
func TestWithFourCleanUpsRunningAFifthUploadWaitsThenFinishes(t *testing.T) {
	admin, ada := labLearner(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" })
	path := ada.createWork("Kedai")
	ada.application.SetCleanUpWaitLog(100 * time.Millisecond)

	release := ada.application.HoldCleanUps(4)
	done := make(chan int)
	go func() {
		res, _ := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
		done <- res.StatusCode
	}()
	select {
	case code := <-done:
		t.Fatalf("a fifth upload finished (%d) while 4 Clean-ups ran", code)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	if code := <-done; code != http.StatusSeeOther {
		t.Fatalf("the fifth upload answered %d after waiting, want 303", code)
	}
	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Beras")

	_, body = admin.get("/admin/log")
	if n := strings.Count(body, waitedLine); n != 1 {
		t.Fatalf("the System log says %d times that an upload waited, want once: %s", n, body)
	}
	expectAll(t, body, "An upload waited")

	// An upload that never waits is not logged.
	ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Gula", 80))
	if _, body = admin.get("/admin/log"); strings.Count(body, waitedLine) != 1 {
		t.Fatalf("an upload that did not wait was logged: %s", body)
	}
}

func TestTheUploadFormShowsTheFileIsBeingRead(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "")
	expectInOrder(t, body, `action="`+path+`/dataset" enctype="multipart/form-data" data-busy`, "cave-loader", "Reading your file")
	expectAll(t, body, `src="/static/forms.js`)
	ada.switchLanguage("ms", path)
	_, body = ada.get(path)
	expectAll(t, body, "Sedang membaca fail anda")
}
