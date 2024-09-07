package routes

import (
	"fmt"
	"net/http"

	"github.com/omareloui/financial-plans/internal/handler"
	"github.com/omareloui/financial-plans/internal/response_handler"
)

func Router() http.Handler {
	rh := response_handler.NewResponseHandler()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", rh.Handler(handler.HomePageHandler))

	mux.HandleFunc("GET /500", rh.Handler(handler.InternalServerErrorHandler))

	static(mux, []string{"styles", "js"}, "./web/public")

	mux.HandleFunc("/", rh.Handler(handler.NotFoundHandler))

	return mux
}

func static(mux *http.ServeMux, paths []string, fspath string) {
	fs := http.FileServer(http.Dir(fspath))
	for _, prefix := range paths {
		mux.Handle(fmt.Sprintf("GET /%s/{path...}", prefix), http.StripPrefix("/", fs))
	}
}
