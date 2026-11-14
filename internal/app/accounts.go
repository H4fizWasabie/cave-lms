package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie         = "__Host-session"
	caveTextSessionCookie = "__Host-cavetext-session" // the CaveText host's own, so a login there is not a login on CAVELMS (ADR 0028)
	sessionLifetime       = 30 * 24 * time.Hour
	verifyLinkLifetime    = 24 * time.Hour
	purposeVerifyEmail    = "verify_email"
	purposeResetPassword  = "reset_password"
	resetLinkLifetime     = time.Hour
)

type learner struct {
	ID      int64
	Name    string
	IsAdmin bool
	IsOwner bool   // the Admin whose email is OWNER_EMAIL
	Lang    string // the saved language, or "" before the Learner first switches
	// PictureKey names the Learner's profile picture in storage, "" when they have none.
	PictureKey string
	// Username stands for the Learner in links (ADR 0023); their Name never does.
	Username string
}

// learnerKey keys the per-request memo of the session lookup that ServeHTTP installs.
type learnerKey struct{}

// currentLearner returns the logged-in Learner, or nil for a Visitor.
// The session is looked up at most once per request, however many handlers ask.
func (a *App) currentLearner(r *http.Request) (*learner, error) {
	if lookup, ok := r.Context().Value(learnerKey{}).(func() (*learner, error)); ok {
		return lookup()
	}
	return a.lookUpLearner(r)
}

// learnerColumns selects a Learner as the session lookups read them; $2 is the owner's email.
const learnerColumns = `l.id, l.name, l.is_admin, l.is_admin AND $2 <> '' AND lower(l.email) = lower($2), coalesce(l.language, ''), l.picture_key, l.username`

// sessionCookieName is the session cookie of the host r asks for. Both are host-only, and each host reads only its own,
// so a session made on one never counts on the other even where a client sends both.
func (a *App) sessionCookieName(r *http.Request) string {
	if a.onCaveTextHost(r) {
		return caveTextSessionCookie
	}
	return sessionCookie
}

func (a *App) lookUpLearner(r *http.Request) (*learner, error) {
	cookie, err := r.Cookie(a.sessionCookieName(r))
	if err != nil {
		return nil, nil
	}
	var l learner
	err = a.db.QueryRowContext(r.Context(), `
		SELECT `+learnerColumns+`
		FROM sessions s JOIN learners l ON l.id = s.learner_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(cookie.Value), a.ownerEmail).Scan(&l.ID, &l.Name, &l.IsAdmin, &l.IsOwner, &l.Lang, &l.PictureKey, &l.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up session: %w", err)
	}
	return &l, nil
}

func (a *App) registerForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "register", page{Next: a.verifyNext(r, r.URL.Query().Get("next"))})
}

// verifyNext is next if it is the connect page of a listed extension on the CaveText host, else "": the only next a
// registration carries through its emails, so a newcomer from the extension ends signed in to it.
func (a *App) verifyNext(r *http.Request, next string) string {
	if localPath(next) == "" || a.connectTarget(r, next) == "" {
		return ""
	}
	return next
}

// mailSite is whose name an email goes out in and where its links point: CAVELMS's, or the CaveText website's for a
// request on its host (ADR 0028). Its catalog keys are CAVELMS's with "cavetext." in front, and its base carries the
// Malay pages' /ms.
type mailSite struct{ keys, base string }

func (a *App) mailSite(r *http.Request) mailSite {
	if a.onCaveTextHost(r) {
		return mailSite{"cavetext.", a.caveTextBaseURL + a.sitePrefix(r)}
	}
	return mailSite{"", a.publicBaseURL}
}

func (m mailSite) text(lang, key string, args ...any) string {
	return text.T(lang, m.keys+key, args...)
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostFormValue("name"))
	username := strings.TrimSpace(r.PostFormValue("username"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	next := a.verifyNext(r, r.PostFormValue("next"))
	form := page{Name: name, Username: username, Email: email, Next: next}
	if form.ErrorField, form.Error = accountFieldError(lang, name, email, password); form.Error == "" {
		if key := usernameError(username); key != "" {
			form.ErrorField, form.Error = "username", text.T(lang, key)
		}
	}
	if form.Error != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "register", form)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// Hashed on every path, though only an unverified used email needs it, so the time does not tell which path ran.
	unknownHash, err := unknownPasswordHash()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	// The Username is checked first, as the insert below skips it when the email has an account: a taken Username is
	// refused the same whether or not the email has one.
	var usernameTaken bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM learners WHERE username = $1)`, username).Scan(&usernameTaken); err != nil {
		a.serverError(w, r, err)
		return
	}
	if usernameTaken {
		form.ErrorField, form.Error = "username", text.T(lang, "account.err.username-taken")
		a.render(w, r, http.StatusConflict, "register", form)
		return
	}
	var id int64
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO learners (name, username, email, password_hash) VALUES ($1, $2, $3, $4) ON CONFLICT ((lower(email))) DO NOTHING RETURNING id`,
		name, username, email, string(hash)).Scan(&id)
	// Commit only once the email is sent, so a failed send leaves no half-made account or dead link.
	switch {
	case isUniqueViolation(err): // the email's conflict is skipped above, so this is the Username, taken since the check
		form.ErrorField, form.Error = "username", text.T(lang, "account.err.username-taken")
		a.render(w, r, http.StatusConflict, "register", form)
		return
	case errors.Is(err, sql.ErrNoRows):
		// The email already has an account: answer as for a new one and email its owner, so the page tells nobody.
		err = a.mailOwnerOfUsedEmail(r, tx, email, unknownHash)
	case err == nil:
		err = a.sendVerification(r.Context(), tx, a.mailSite(r), lang, id, email, next)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:   text.T(lang, "message.check-email"),
		Message: text.T(lang, "register.sent", email),
	})
}

// sendVerification replaces the Learner's verification link with a new one and emails it in lang, if email may have one.
func (a *App) sendVerification(ctx context.Context, tx *sql.Tx, site mailSite, lang string, learnerID int64, email, next string) error {
	if !a.verificationAllowed(email) {
		return nil
	}
	token, err := issueOneTimeToken(ctx, tx, learnerID, purposeVerifyEmail, verifyLinkLifetime)
	if err != nil {
		return err
	}
	return a.mailVerification(ctx, site, lang, email, token, next)
}

// mailOwnerOfUsedEmail answers a registration with email, which already has an account: an unverified account gets a link
// to set its password, a verified one a notice that someone tried to register with its address. unknownHash is the
// password the unverified account is given.
func (a *App) mailOwnerOfUsedEmail(r *http.Request, tx *sql.Tx, email, unknownHash string) error {
	var (
		id        int64
		to, saved string
		verified  bool
	)
	if err := tx.QueryRowContext(r.Context(),
		`SELECT id, email, coalesce(language, ''), verified_at IS NOT NULL FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &to, &saved, &verified); err != nil {
		return fmt.Errorf("look up account for used email: %w", err)
	}
	lang, err := a.recipientLanguage(r, saved)
	if err != nil {
		return err
	}
	site := a.mailSite(r)
	if !verified {
		// An unverified account's password proves nothing about who chose it, so whoever holds the inbox sets it with a
		// reset link, which also confirms the email. A verification link would confirm the first registrant's password,
		// so the ones already sent stop working and the password goes: replaced by a random one nobody knows, a real
		// bcrypt hash so a login to it takes as long as to any account. This comes before the cap on emails below, so the
		// first registrant cannot keep the password by using the cap up.
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM one_time_tokens WHERE learner_id = $1 AND purpose = $2`, id, purposeVerifyEmail); err != nil {
			return fmt.Errorf("delete verification links: %w", err)
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE learners SET password_hash = $1 WHERE id = $2`, unknownHash, id); err != nil {
			return fmt.Errorf("replace unverified password: %w", err)
		}
	}
	if !a.verificationAllowed(to) {
		return nil
	}
	if !verified {
		token, err := issueOneTimeToken(r.Context(), tx, id, purposeResetPassword, resetLinkLifetime)
		if err != nil {
			return err
		}
		return a.mailer.Send(r.Context(), mailer.Message{
			To:      to,
			Subject: site.text(lang, "email.unverified-again.subject"),
			Body:    site.text(lang, "email.unverified-again.body", site.base+"/reset-password?token="+token),
		})
	}
	return a.mailer.Send(r.Context(), mailer.Message{
		To:      to,
		Subject: site.text(lang, "email.used.subject"),
		Body:    site.text(lang, "email.used.body", site.base+"/login", site.base+"/forgot-password"),
	})
}

// verificationAllowed counts an email to email, a verification, a set-password link or a used-email notice, and reports
// whether it is within verificationsPerAddress in verificationWindow. Past that, callers neither send nor replace the
// link, and answer as if they had, so the requester learns nothing and the link already sent keeps working.
func (a *App) verificationAllowed(email string) bool {
	return a.limiter.allow("verification email "+strings.ToLower(email), verificationsPerAddress, verificationWindow, a.now())
}

// mailVerification emails the verification link for token in lang, which carries next on to the verified page.
func (a *App) mailVerification(ctx context.Context, site mailSite, lang, email, token, next string) error {
	link := site.base + "/verify?token=" + token
	if next != "" {
		link += "&next=" + url.QueryEscape(next)
	}
	return a.mailer.Send(ctx, mailer.Message{
		To:      email,
		Subject: site.text(lang, "email.verify.subject"),
		Body:    site.text(lang, "email.verify.body", link),
	})
}

// mailLater runs send, which sends one email, after the request has answered, so a slow mail service never makes an
// account that exists answer later than one that does not. A failed send is only logged (job says what it was): an
// error page would reveal that the account exists. Close waits for it, and its context does not end with the request.
// ponytail: the server does not call Close on SIGTERM, so a send in flight during a restart is lost, as it was in the
// request, though the page has said it went; the Learner asks again. Stop gracefully in main if that ever matters.
func (a *App) mailLater(r *http.Request, job string, send func(ctx context.Context) error) {
	method, path := r.Method, r.URL.Path
	a.mailingLater.Add(1)
	go func() {
		defer a.mailingLater.Done()
		a.survive(job, func() {
			if err := send(context.Background()); err != nil {
				a.writeLog("error", fmt.Errorf("%s: %w", job, err).Error(), method, path)
			}
		})
	}()
}

// recipientLanguage is the language to email a Learner in: their saved one, else the request's. On the CaveText
// website it is always the page's, which ignores the saved one.
func (a *App) recipientLanguage(r *http.Request, saved string) (string, error) {
	if text.Supported(saved) && !a.onCaveTextHost(r) {
		return saved, nil
	}
	return a.language(r)
}

// issueOneTimeToken replaces the Learner's links for purpose with a new one and returns the token to email.
func issueOneTimeToken(ctx context.Context, tx *sql.Tx, learnerID int64, purpose string, lifetime time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM one_time_tokens WHERE learner_id = $1 AND purpose = $2`, learnerID, purpose); err != nil {
		return "", fmt.Errorf("delete old %s links: %w", purpose, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO one_time_tokens (token_hash, learner_id, purpose, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))`,
		hashToken(token), learnerID, purpose, lifetime.Seconds()); err != nil {
		return "", fmt.Errorf("store %s link: %w", purpose, err)
	}
	return token, nil
}

// verifyForm is the page a verification link opens. It changes nothing and keeps the link, as mail scanners and link
// previews open every link in an email: the button posts the token to verify.
func (a *App) verifyForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "verify_confirm", page{Token: r.URL.Query().Get("token"), Next: a.verifyNext(r, r.URL.Query().Get("next"))})
}

func (a *App) verify(w http.ResponseWriter, r *http.Request) {
	var (
		learnerID int64
		email     string
		expired   bool
	)
	// Deleting the token as it is read makes every link single-use.
	err := a.db.QueryRowContext(r.Context(), `
		DELETE FROM one_time_tokens t USING learners l
		WHERE t.learner_id = l.id AND t.token_hash = $1 AND t.purpose = $2
		RETURNING l.id, l.email, t.expires_at <= now()`,
		hashToken(r.PostFormValue("token")), purposeVerifyEmail).Scan(&learnerID, &email, &expired)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	lang, langErr := a.language(r)
	if langErr != nil {
		a.serverError(w, r, langErr)
		return
	}
	next := a.verifyNext(r, r.PostFormValue("next")) // on to the login that follows, or the resend form
	// Commit only once the email is sent, so a failed send leaves no half-made account or dead link.
	switch {
	case errors.Is(err, sql.ErrNoRows):
		a.render(w, r, http.StatusBadRequest, "message", page{
			Title:       text.T(lang, "message.link-invalid"),
			Message:     text.T(lang, "verify.invalid"),
			OfferResend: true,
			Next:        next,
		})
	case expired:
		a.render(w, r, http.StatusGone, "message", page{
			Title:       text.T(lang, "message.link-expired"),
			Message:     text.T(lang, "verify.expired"),
			OfferResend: true,
			Email:       email,
			Next:        next,
		})
	default:
		if _, err := a.db.ExecContext(r.Context(), `UPDATE learners SET verified_at = now() WHERE id = $1 AND verified_at IS NULL`, learnerID); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusOK, "message", page{Title: text.T(lang, "verify.done.title"), Message: text.T(lang, "verify.done"), OfferLogin: true, Next: next})
	}
}

func (a *App) resendVerification(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	next := a.verifyNext(r, r.PostFormValue("next"))
	site := a.mailSite(r)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var (
		id        int64
		to, saved string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, email, coalesce(language, '') FROM learners WHERE lower(email) = lower($1) AND verified_at IS NULL`, email).Scan(&id, &to, &saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	if err == nil && a.verificationAllowed(to) {
		token, err := issueOneTimeToken(r.Context(), tx, id, purposeVerifyEmail, verifyLinkLifetime)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if err := tx.Commit(); err != nil {
			a.serverError(w, r, err)
			return
		}
		lang, err := a.recipientLanguage(r, saved)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.mailLater(r, "send verification email", func(ctx context.Context) error {
			return a.mailVerification(ctx, site, lang, to, token, next)
		})
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:   text.T(lang, "message.check-email"),
		Message: text.T(lang, "verify.resent", email),
	})
}

func (a *App) loginForm(w http.ResponseWriter, r *http.Request) {
	next := localPath(r.URL.Query().Get("next"))
	a.allowConnectRedirect(w, r, next)
	a.render(w, r, http.StatusOK, "login", page{Next: next})
}

// localPath returns next if it is a path on this site, else "", so a login link can't send anyone off-site.
// Control characters are refused outright: browsers drop tabs and newlines, so "/\t/evil.com" becomes "//evil.com".
// So is any backslash: browsers read it as "/", and http.Redirect cleans "/./\evil.com" down to "/\evil.com".
func localPath(next string) string {
	if strings.ContainsFunc(next, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") ||
		strings.HasPrefix(next, "//") || strings.ContainsRune(next, '\\') {
		return ""
	}
	return next
}

// dummyHash is compared against when a login's email has no account.
var dummyHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("no account has this password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
}()

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	next := localPath(r.PostFormValue("next"))
	a.allowConnectRedirect(w, r, next)
	// Every attempt is counted first; a right password takes its count back, so only failures stay and a class sharing
	// one address all gets in while guessing is still stopped.
	account, address, emailKey := loginKeys(r, email)
	now := time.Now()
	if ok, report := a.limiter.allowLogin(account, address, emailKey, now); !ok {
		if report {
			// %q, as the email is whatever the client sent and the System log stores only valid text.
			a.logInfo("Login limit per email: %q refused after %d failed logins from all addresses within %v",
				maskEmail(email), loginFailuresPerEmail, loginEmailWindow)
		}
		a.tooManyAttempts(w, r)
		return
	}
	var (
		id       int64
		hash     string
		verified bool
		saved    string
	)
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash, verified_at IS NOT NULL, coalesce(language, '') FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &hash, &verified, &saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	if err != nil {
		hash = string(dummyHash) // compare anyway, so a missing account takes as long as a wrong password
	}
	// The compare runs before err is checked on purpose, so a missing account costs the same time.
	failed := bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.PostFormValue("password"))) != nil || err != nil
	if !failed {
		a.limiter.forget(account)
		a.limiter.forget(address)
		a.limiter.forget(emailKey)
	}
	if failed || !verified {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if failed {
			a.render(w, r, http.StatusUnauthorized, "login", page{Email: email, Error: text.T(lang, "login.err.wrong"), Next: next})
			return
		}
		a.render(w, r, http.StatusForbidden, "message", page{
			Title:       text.T(lang, "login.unverified.title"),
			Message:     text.T(lang, "login.unverified"),
			OfferResend: true,
			Email:       email,
			Next:        a.verifyNext(r, next),
		})
		return
	}

	token, err := newToken()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO sessions (token_hash, learner_id, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3))`,
		hashToken(token), id, sessionLifetime.Seconds()); err != nil {
		a.serverError(w, r, err)
		return
	}
	setSessionCookie(w, a.sessionCookieName(r), token, int(sessionLifetime.Seconds()))
	if saved != "" && !a.onCaveTextHost(r) {
		setLanguageCookie(w, saved) // so the site stays in it after logging out
	}
	// On the CaveText host, a login that came from the extension's connect page is the Learner's consent: the token goes
	// to the extension at once, with no Connect button in between.
	if ext := a.connectTarget(r, next); ext != "" {
		a.connectExtension(w, r, ext, id)
		return
	}
	// Without a next, everyone lands on the home page, Learner and Admin alike.
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	name := a.sessionCookieName(r)
	if cookie, err := r.Cookie(name); err == nil {
		if _, err := a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = $1`, hashToken(cookie.Value)); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	setSessionCookie(w, name, "", -1)
	http.Redirect(w, r, a.sitePrefix(r)+"/", http.StatusSeeOther) // the CaveText website's home is its static page
}

func setSessionCookie(w http.ResponseWriter, name, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// accountFieldError returns the first invalid field of name, email and password, and a message in lang saying why,
// or "", "".
func accountFieldError(lang, name, email, password string) (field, msg string) {
	switch {
	case name == "" || len(name) > 200:
		return "name", text.T(lang, "account.err.name")
	case !validEmail(email):
		return "email", text.T(lang, "account.err.email")
	}
	if msg := passwordProblem(lang, password); msg != "" {
		return "password", msg
	}
	return "", ""
}

// passwordProblem says in lang why a password is unacceptable, or returns "" if it is fine.
func passwordProblem(lang, password string) string {
	switch {
	case len(password) < 8:
		return text.T(lang, "account.err.password-short")
	case len(password) > 72: // bcrypt's limit
		return text.T(lang, "account.err.password-long")
	}
	return ""
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && len(email) <= 254
}

// unknownPasswordHash is a bcrypt hash of a fresh random token: a password nobody knows.
func unknownPasswordHash() (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	return string(hash), err
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
