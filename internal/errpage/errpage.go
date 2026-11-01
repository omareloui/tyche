// Package errpage writes error responses: a full page for browser
// navigations (incl. HTMX boosted ones), the plain message for other HTMX
// requests, which swap it inline (e.g. a form's 422 message).
package errpage

import (
	"net/http"

	"github.com/omareloui/tyche/internal/reqpath"
	"github.com/omareloui/tyche/web/views"
)

// Write responds with the error d.Code, d.Detail being its message. The
// message of a 5xx is never shown, it's for the logs only.
func Write(w http.ResponseWriter, r *http.Request, d views.ErrorPageData) {
	if d.Detail == http.StatusText(d.Code) || d.Code >= 500 {
		d.Detail = ""
	}

	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {
		msg := d.Detail
		if msg == "" {
			msg = http.StatusText(d.Code)
		}
		http.Error(w, msg, d.Code)
		return
	}

	h := w.Header()
	h.Del("Content-Length")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(d.Code)
	_ = views.ErrorPage(d).Render(reqpath.With(r.Context(), r.URL.Path), w)
}
