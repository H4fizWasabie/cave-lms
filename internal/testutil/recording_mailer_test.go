package testutil

import (
	"context"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

func TestRecordingMailerRecordsSentMessages(t *testing.T) {
	recorder := &RecordingMailer{}
	want := mailer.Message{To: "learner@example.test", Subject: "Verify your email", Body: "https://courses.example.test/verify/token"}
	if err := recorder.Send(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got := recorder.Messages()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Messages() = %#v, want one message %#v", got, want)
	}
	got[0].Body = "changed"
	if recorder.Messages()[0] != want {
		t.Fatal("Messages() exposed the recording's internal slice")
	}
}
