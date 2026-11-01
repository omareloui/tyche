package errpage_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/errpage"
	"github.com/omareloui/tyche/web/views"
)

func write(headers map[string]string, d views.ErrorPageData) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	errpage.Write(rec, r, d)
	return rec
}

func TestPageForNavigations(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"browser": nil,
		"boosted": {"HX-Request": "true", "HX-Boosted": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := write(headers, views.ErrorPageData{Code: http.StatusNotFound, Detail: "sms template not found"})
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("content type = %q, want html", ct)
			}
			body := rec.Body.String()
			for _, want := range []string{"<!doctype html>", "404", "This page wandered off", "sms template not found"} {
				if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
					t.Errorf("body lacks %q", want)
				}
			}
		})
	}
}

func TestPlainMessageForHTMX(t *testing.T) {
	rec := write(map[string]string{"HX-Request": "true"}, views.ErrorPageData{Code: http.StatusUnprocessableEntity, Detail: "Select what to delete."})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Select what to delete." {
		t.Errorf("body = %q, want the plain message", got)
	}

	rec = write(map[string]string{"HX-Request": "true"}, views.ErrorPageData{Code: http.StatusForbidden})
	if got := strings.TrimSpace(rec.Body.String()); got != "Forbidden" {
		t.Errorf("body = %q, want the status text", got)
	}
}

func TestServerErrorHidesMessage(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"browser": nil,
		"htmx":    {"HX-Request": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := write(headers, views.ErrorPageData{Code: http.StatusInternalServerError, Detail: "db secret", Ref: "REF123"})
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "db secret") {
				t.Error("body leaks the error message")
			}
			if name == "browser" && !strings.Contains(rec.Body.String(), "REF123") {
				t.Error("body lacks the reference")
			}
		})
	}
}
