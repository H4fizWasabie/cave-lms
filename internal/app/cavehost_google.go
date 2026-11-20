package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// maxSheetListSize caps the sheet list page read from Google (about 70 KB for a small Spreadsheet).
const maxSheetListSize = 5 << 20

// A CaveHost Spreadsheet is fetched from Google by Cave, as one xlsx file, on the server (ADR 0025). The fetch is cut off
// after spreadsheetTimeout, and a file over maxSpreadsheetSize is refused, so one huge Spreadsheet cannot take the VPS down.
const (
	spreadsheetTimeout = 60 * time.Second
	maxSpreadsheetSize = 50 << 20
	maxRedirects       = 10
)

// googleSheetsURL is where Spreadsheets are fetched from unless the config says another place, which only tests do.
const googleSheetsURL = "https://docs.google.com"

var (
	// errSpreadsheetClosed is fetchSpreadsheet's error when Cave cannot open the Spreadsheet: Google answers no, sends Cave to
	// a sign-in or another site, or sends something that is not an xlsx file. The Learner has to share it as "anyone with the link".
	errSpreadsheetClosed = errors.New("Cave cannot open the Spreadsheet")
	// errSpreadsheetTooBig is fetchSpreadsheet's error for a file over maxSpreadsheetSize.
	errSpreadsheetTooBig = errors.New("the Spreadsheet is over the size cap")
)

// spreadsheetPath is the path of a Spreadsheet in a link, with its ID: letters, digits, dashes and underscores.
var spreadsheetPath = regexp.MustCompile(`^/spreadsheets/d/([A-Za-z0-9_-]{1,200})(?:/|$)`)

// spreadsheetOf reads a pasted link. Only https://docs.google.com/spreadsheets/d/<id>/... is accepted; it returns the
// Spreadsheet's ID and the gid of the sheet in the link, "" for none. Nothing else is read from the link, and it is never fetched.
func spreadsheetOf(link string) (id, gid string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "docs.google.com") || u.Port() != "" || u.User != nil {
		return "", "", false
	}
	m := spreadsheetPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	gid = u.Query().Get("gid")
	if frag, err := url.ParseQuery(u.Fragment); err == nil && frag.Get("gid") != "" {
		gid = frag.Get("gid")
	}
	if len(gid) > 20 || strings.Trim(gid, "0123456789") != "" {
		gid = ""
	}
	return m[1], gid, true
}

// newSpreadsheetClient is the client that fetches Spreadsheets from the Google at base. It follows redirects only to that
// Google, docs.google.com and *.googleusercontent.com, where Google serves the file, and never to another host.
func newSpreadsheetClient(base *url.URL) *http.Client {
	return &http.Client{
		Timeout: spreadsheetTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			host := strings.ToLower(req.URL.Hostname())
			switch {
			case len(via) >= maxRedirects:
				return errors.New("too many redirects")
			case req.URL.Host == base.Host && req.URL.Scheme == base.Scheme:
			case req.URL.Scheme == "https" && req.URL.Port() == "" && (host == "docs.google.com" || strings.HasSuffix(host, ".googleusercontent.com")):
			default:
				return errSpreadsheetClosed
			}
			return nil
		},
	}
}

// fetchSpreadsheet downloads the Spreadsheet with this ID as an xlsx file. Cave builds the address itself from the ID.
// errSpreadsheetClosed and errSpreadsheetTooBig are for the Learner to act on; any other error is Google or the network failing.
func (a *App) fetchSpreadsheet(ctx context.Context, id string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.googleSheetsURL+"/spreadsheets/d/"+url.PathEscape(id)+"/export?format=xlsx", nil)
	if err != nil {
		return nil, err
	}
	res, err := a.spreadsheets.Do(req)
	if errors.Is(err, errSpreadsheetClosed) {
		return nil, errSpreadsheetClosed
	}
	if err != nil {
		return nil, fmt.Errorf("fetch Spreadsheet: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode >= 500:
		return nil, fmt.Errorf("fetch Spreadsheet: Google answered %s", res.Status)
	case res.StatusCode != http.StatusOK:
		return nil, errSpreadsheetClosed
	case res.ContentLength > maxSpreadsheetSize:
		return nil, errSpreadsheetTooBig
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxSpreadsheetSize+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("fetch Spreadsheet: %w", err)
	case len(data) > maxSpreadsheetSize:
		return nil, errSpreadsheetTooBig
	case !bytes.HasPrefix(data, []byte("PK")): // an xlsx file is a zip; a sign-in page is not
		return nil, errSpreadsheetClosed
	}
	return data, nil
}

// sheetListItem is one sheet in Google's public sheet list, the Spreadsheet's htmlview page: its script has an
// items.push({name: "...", pageUrl: "...", gid: "...", ...}) per sheet, the name a JavaScript string. The gid is there and in
// no xlsx file. The pattern is loose on purpose: the other keys, their order and the spacing may change without breaking it.
var sheetListItem = regexp.MustCompile(`items\.push\(\{\s*name:\s*"((?:[^"\\]|\\.)*)"[^}]*?\bgid:\s*"(-?[0-9]+)"`)

// jsHexEscape finds \x3d, a JavaScript string escape that JSON does not have (nor \', which sheetNameOfGid also reads).
var jsHexEscape = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

// sheetNameOfGid asks Google's public sheet list of the Spreadsheet with this ID for the name of the sheet with this gid. It is
// only a hint for which sheet starts ticked, so any failure, or a gid that is not in the list, is an error for the caller to ignore.
func (a *App) sheetNameOfGid(ctx context.Context, id, gid string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.googleSheetsURL+"/spreadsheets/d/"+url.PathEscape(id)+"/htmlview", nil)
	if err != nil {
		return "", err
	}
	res, err := a.spreadsheets.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch sheet list: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch sheet list: Google answered %s", res.Status)
	}
	page, err := io.ReadAll(io.LimitReader(res.Body, maxSheetListSize))
	if err != nil {
		return "", fmt.Errorf("fetch sheet list: %w", err)
	}
	for _, m := range sheetListItem.FindAllSubmatch(page, -1) {
		if string(m[2]) != gid {
			continue
		}
		// A sheet name cannot hold a backslash, so every backslash in it starts an escape.
		lit := strings.ReplaceAll(jsHexEscape.ReplaceAllString(string(m[1]), `\u00$1`), `\'`, `'`)
		var name string
		if err := json.Unmarshal([]byte(`"`+lit+`"`), &name); err != nil {
			return "", fmt.Errorf("read sheet list: %w", err)
		}
		return name, nil
	}
	return "", errors.New("the gid is not in the sheet list")
}
