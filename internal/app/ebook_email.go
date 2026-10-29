package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// ebookAccessSQL is the cavetext_pro_periods condition for Ebook access running now (ADR 0020).
const ebookAccessSQL = `ebooks AND starts_at <= now() AND ends_at > now() AND ended_early_at IS NULL`

// maxEbookAttachments is the most raw ebook bytes the Ebook email carries: Gmail refuses over 25 MB and base64 grows files by a third.
const maxEbookAttachments = 18 * 1024 * 1024

// emailEbookLink sends the Learner the Ebook email: a link to the Gallery's ebooks and the day their Ebook access ends,
// in their saved language, with a copy of each ebook that is not Promotional material (ebookAttachments). Callers send
// it once their transaction has committed. A failed email is only logged: the access is given, and the Learner finds
// the ebooks in the Gallery.
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
	if files := a.ebookAttachments(ctx, learnerID); len(files) > 0 {
		withFiles := msg
		withFiles.Body += "\n" + text.T(lang, "email.ebook.attached") + "\n"
		withFiles.Attachments = files
		err := a.mailer.Send(ctx, withFiles)
		if err == nil {
			return
		}
		// A large upload can fail where a small one would not, so the link alone is sent rather than nothing.
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d with the ebooks attached, sending the link only: %w", learnerID, err))
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: %w", learnerID, err))
	}
}

// ebookAttachments reads every ebook in the Gallery that is not Promotional material, whatever its Visibility. It
// returns none, after logging why, when they are over maxEbookAttachments together or a file cannot be read: the
// email then goes with the link alone.
func (a *App) ebookAttachments(ctx context.Context, learnerID int64) []mailer.Attachment {
	rows, err := a.db.QueryContext(ctx, `SELECT id, title, storage_key, size_bytes FROM gallery_items
		WHERE format = 'ebook' AND NOT promotional ORDER BY id`)
	if err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: list ebooks, link only: %w", learnerID, err))
		return nil
	}
	defer rows.Close()
	var files []mailer.Attachment
	var keys []string
	var total int64
	for rows.Next() {
		var id, size int64
		var title, key string
		if err := rows.Scan(&id, &title, &key, &size); err != nil {
			a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: list ebooks, link only: %w", learnerID, err))
			return nil
		}
		total += size
		keys = append(keys, key)
		files = append(files, mailer.Attachment{Filename: cleanFileName(strings.NewReplacer("/", "-", `\`, "-").Replace(title)) + ".pdf"})
	}
	if err := rows.Err(); err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: list ebooks, link only: %w", learnerID, err))
		return nil
	}
	if total > maxEbookAttachments {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: the ebooks total %d bytes, over the %d byte limit, so the link only was sent", learnerID, total, maxEbookAttachments))
		return nil
	}
	for i, key := range keys {
		if files[i].Content, err = a.store.Get(ctx, key); err != nil {
			a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: read ebook file, link only: %w", learnerID, err))
			return nil
		}
	}
	return files
}
