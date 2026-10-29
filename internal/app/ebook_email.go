package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// ebookAccessSQL is the cavetext_pro_periods condition for Ebook access running now (ADR 0020).
const ebookAccessSQL = `ebooks AND starts_at <= now() AND ends_at > now() AND ended_early_at IS NULL`

// ebookLinkDays is how long an Ebook download link works.
const ebookLinkDays = 7

// emailEbookLink sends the Learner the Ebook email: a link to the Gallery's ebooks, the day their Ebook access ends and
// an Ebook download link for each ebook that is not Promotional material (ebookDownloadLinks), in their saved language.
// Callers send it once their transaction has committed. A failed email is only logged: the access is given, and the
// Learner finds the ebooks in the Gallery.
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
	body := text.T(lang, "email.ebook.body", name, text.Date(lang, until.Time.UTC()), a.publicBaseURL+"/gallery?format=ebook", email)
	if links := a.ebookDownloadLinks(ctx, learnerID); links != "" {
		body += "\n" + text.T(lang, "email.ebook.links", ebookLinkDays, links)
	}
	if err := a.mailer.Send(ctx, mailer.Message{To: email, Subject: text.T(lang, "email.ebook.subject"), Body: body}); err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: %w", learnerID, err))
	}
}

// ebookDownloadLinks makes a link for each ebook in the Gallery that is not Promotional material, whatever its
// Visibility, and returns them one per line as "title: address". A failure is logged and the ebooks it hit are left
// out, so the email still goes with the Gallery link.
func (a *App) ebookDownloadLinks(ctx context.Context, learnerID int64) string {
	rows, err := a.db.QueryContext(ctx, `SELECT id, title FROM gallery_items WHERE format = 'ebook' AND NOT promotional ORDER BY id`)
	if err != nil {
		a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: list ebooks, no download links: %w", learnerID, err))
		return ""
	}
	type ebook struct {
		id    int64
		title string
	}
	var ebooks []ebook
	for rows.Next() {
		var e ebook
		if err := rows.Scan(&e.id, &e.title); err != nil {
			rows.Close()
			a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: list ebooks, no download links: %w", learnerID, err))
			return ""
		}
		ebooks = append(ebooks, e)
	}
	rows.Close()
	var lines strings.Builder
	for _, e := range ebooks {
		token, err := newToken()
		if err == nil {
			_, err = a.db.ExecContext(ctx, `INSERT INTO ebook_download_links (learner_id, gallery_item_id, token_hash, expires_at)
				VALUES ($1, $2, $3, now() + make_interval(days => $4))`, learnerID, e.id, hashToken(token), ebookLinkDays)
		}
		if err != nil {
			a.logError(nil, fmt.Errorf("send Ebook email to Learner %d: make download link for ebook %d, left out: %w", learnerID, e.id, err))
			continue
		}
		fmt.Fprintf(&lines, "%s: %s/ebook-downloads/%s\n", e.title, a.publicBaseURL, token)
	}
	return lines.String()
}

// ebookDownload serves the ebook an Ebook download link names, with no sign-in (ADR 0020). A link that is unknown
// answers 404; one that has expired, or whose ebook is now Promotional material or whose Learner's Ebook access has
// ended, answers 410. Both send the Learner to the Gallery and the Admin.
func (a *App) ebookDownload(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.allow("ebook download "+clientAddress(r), ebookDownloadLookups, rateLimitWindow, a.now()) {
		a.tooManyAttempts(w, r)
		return
	}
	var title, key, mimeType string
	var works bool
	err := a.db.QueryRowContext(r.Context(), `SELECT g.title, g.storage_key, g.mime_type,
		l.expires_at > now() AND g.format = 'ebook' AND NOT g.promotional
		AND EXISTS (SELECT 1 FROM cavetext_pro_periods WHERE learner_id = l.learner_id AND `+ebookAccessSQL+`)
		FROM ebook_download_links l JOIN gallery_items g ON g.id = l.gallery_item_id WHERE l.token_hash = $1`,
		hashToken(r.PathValue("token"))).Scan(&title, &key, &mimeType, &works)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	if err != nil || !works {
		status := http.StatusGone
		if err != nil {
			status = http.StatusNotFound
		}
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, status, "message", page{Title: text.T(lang, "ebook.link.gone.title"), Message: text.T(lang, "ebook.link.gone"), OfferGallery: true})
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, cleanFileName(strings.NewReplacer("/", "-", `\`, "-", `"`, "'").Replace(title))))
	w.Header().Set("Cache-Control", "private, no-store")
	a.serveStored(w, r, key, mimeType)
}
