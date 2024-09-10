package router

import (
	"fmt"
	"net/http"

	"github.com/omareloui/financial-plans/internal/handler"
	"github.com/omareloui/financial-plans/internal/response_handler"
)

type Router interface {
	http.Handler
}

func New(h *handler.Handler, rh *response_handler.ResponseHandler) Router {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", rh.Handler(h.HomePageHandler))

	static(mux, []string{"styles", "js"}, "./web/public")

	mux.HandleFunc("/", rh.Handler(h.NotFoundHandler))

	return mux
}

func static(mux *http.ServeMux, paths []string, fspath string) {
	fs := http.FileServer(http.Dir(fspath))
	for _, prefix := range paths {
		mux.Handle(fmt.Sprintf("GET /%s/{path...}", prefix), http.StripPrefix("/", fs))
	}
}
