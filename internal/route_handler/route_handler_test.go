package route_handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/route_handler"
)

func TestPanicReturns500(t *testing.T) {
	handlers := map[string]http.Handler{
		"templ": route_handler.RouteHandler(func(http.ResponseWriter, *http.Request) (templ.Component, error) {
			panic("boom")
		}),
		"json": route_handler.JSONRouteHandler(func(http.ResponseWriter, *http.Request) ([]byte, error) {
			panic("boom")
		}),
	}

	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
		})
	}
}
