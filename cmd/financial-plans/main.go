package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/omareloui/financial-plans/config"
	"github.com/omareloui/financial-plans/internal/handler"
	"github.com/omareloui/financial-plans/internal/response_handler"
)

func main() {
	port := config.GetApplicationPort()
	rh := response_handler.NewResponseHandler()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", rh.Handler(handler.HomePageHandler))

	static(mux, []string{"styles", "js"}, "./web/public")

	mux.HandleFunc("/", rh.Handler(handler.NotFoundHandler))

	log.Printf("Starting to listen on http://localhost:%d\n", port)
	log.Fatalln(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}

func static(mux *http.ServeMux, paths []string, fspath string) {
	fs := http.FileServer(http.Dir(fspath))
	for _, prefix := range paths {
		mux.Handle(fmt.Sprintf("GET /%s/{path...}", prefix), http.StripPrefix("/", fs))
	}
}
