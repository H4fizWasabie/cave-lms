package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// ebookAccessSQL is the cavetext_pro_periods condition for Ebook access running now (ADR 0020).
const ebookAccessSQL = `ebooks AND starts_at <= now() AND ends_at > now() AND ended_early_at IS NULL`

// emailEbookLink sends the Learner the Ebook email: a link to the Gallery's ebooks and the day their Ebook access ends,
// in their saved language. Callers send it once their transaction has committed. A failed email is only logged: the
// access is given, and the Learner finds the ebooks in the Gallery.
func (a *App) emailEbookLink(ctx context.Context, learnerID int64) {
	var name, email, lang string
	var until sql.NullTime
	err := a.db.QueryRowContext(ctx, `SELECT name, email, COALESCE(language, ''),
		(SELECT max(ends_at) FROM cavetext_pro_periods WHERE learner_id = learners.id AND `+ebookAccessSQL+`)
		FROM learners WHERE id = $1`, learnerID).Scan(&name, &email, &lang, &until)
	if err == nil && !until.Valid {
		err = errors.New("no running Ebook access")
	}
	if err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: %w", learnerID, err))
		return
	}
	if !text.Supported(lang) {
		lang = text.Languages[0]
	}
	msg := mailer.Message{
		To:      email,
		Subject: text.T(lang, "email.ebook.subject"),
		Body:    text.T(lang, "email.ebook.body", name, text.Date(lang, until.Time.UTC()), a.publicBaseURL+"/gallery?format=ebook", email),
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: %w", learnerID, err))
	}
}
