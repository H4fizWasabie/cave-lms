package testutil

import (
	"context"
	"reflect"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

func TestRecordingMailerRecordsSentMessagesWithTheirAttachments(t *testing.T) {
	recorder := &RecordingMailer{}
	want := mailer.Message{To: "learner@example.test", Subject: "Your Result", Body: "You passed.",
		Attachments: []mailer.Attachment{{Filename: "answer-sheet.pdf", Content: []byte("%PDF-1.3")}}}
	if err := recorder.Send(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got := recorder.Messages()
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("Messages() = %#v, want one message %#v", got, want)
	}
	got[0].Body = "changed"
	if recorder.Messages()[0].Body != want.Body {
		t.Fatal("Messages() exposed the recording's internal slice")
	}
}
