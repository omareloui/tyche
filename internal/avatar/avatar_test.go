package avatar

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch(t *testing.T) {
	png := []byte("\x89PNG fake")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
		case "/big":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(make([]byte, MaxSize+1))
		case "/limited":
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()

	pic, err := Fetch(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pic.Data, png) || pic.ContentType != "image/png" || len(pic.Hash) != 16 {
		t.Errorf("got %+v", pic)
	}

	for path, want := range map[string]error{"/html": ErrNotImage, "/big": ErrTooBig, "/limited": ErrBadStatus} {
		if _, err := Fetch(context.Background(), srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", path, err, want)
		}
	}
}
