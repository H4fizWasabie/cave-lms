package app

import (
	"crypto/subtle"
	"io"
	"net/http"
)

// bclWebhook takes BCL's webhooks for the CaveText Pro payment form. BCL signs nothing, so the secret
// key in the address is the only proof the call is BCL's: a wrong or unset key gets the not-found page.
// For now it only keeps the raw body in the System log, so the real payload can be read before Pro is
// granted from it. The log line names the path without the key.
func (a *App) bclWebhook(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if a.bclWebhookKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(a.bclWebhookKey)) != 1 {
		a.notFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	a.writeLog("info", "BCL webhook: "+string(body), r.Method, "/cavetext/bcl/…")
	w.WriteHeader(http.StatusOK)
}
