package handler

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

func (h Handler) NotFound(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.New(http.StatusNotFound, "")
}

func (h Handler) InternalServerError(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return nil, resperr.New(http.StatusInternalServerError, "")
}

// Offline is the page the service worker caches for page loads without a
// network. It never reads the user (not even the claims).
func (h Handler) Offline(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return views.OfflinePage(), nil
}
