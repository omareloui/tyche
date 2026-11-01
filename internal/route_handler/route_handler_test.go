package route_handler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/resperr"
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

func TestErrorsDontLeak(t *testing.T) {
	handlers := map[string]http.Handler{
		"templ": route_handler.RouteHandler(func(http.ResponseWriter, *http.Request) (templ.Component, error) {
			return nil, errors.New("db secret")
		}),
		"json": route_handler.JSONRouteHandler(func(http.ResponseWriter, *http.Request) ([]byte, error) {
			return nil, errors.New("db secret")
		}),
	}

	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			if strings.Contains(rec.Body.String(), "db secret") {
				t.Errorf("body leaks the error: %q", rec.Body.String())
			}
		})
	}
}

func TestJSONErrorBody(t *testing.T) {
	h := route_handler.JSONRouteHandler(func(http.ResponseWriter, *http.Request) ([]byte, error) {
		return nil, resperr.New(http.StatusBadRequest, "invalid json body")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"invalid json body"}` {
		t.Errorf("body = %s", got)
	}
}
