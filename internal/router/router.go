package router

import (
	"fmt"
	"net/http"

	"github.com/omareloui/financial-plans/internal/handler"
	"github.com/omareloui/financial-plans/internal/middleware"
	"github.com/omareloui/financial-plans/internal/route_handler"
)

type Router interface {
	http.Handler
}

func New(h *handler.Handler) Router {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handle(h.HomePage))

	mux.Handle("GET /transactions", handle(h.Transactions))
	mux.Handle("POST /transactions", handle(h.CreateTransaction))
	mux.Handle("POST /transactions/amount-inputs", handle(h.AmountInputs))
	mux.Handle("GET /transactions/{id}", handle(h.Transaction))
	mux.Handle("GET /transactions/{id}/edit", handle(h.GetEditTransaction))
	mux.Handle("PUT /transactions/{id}", handle(h.EditTransaction))

	static(mux, []string{"styles", "js"}, "./web/public")
	mux.Handle("/", handle(h.NotFound))

	return mux
}

func static(mux *http.ServeMux, paths []string, fspath string) {
	fs := http.StripPrefix("/", http.FileServer(http.Dir(fspath)))
	for _, prefix := range paths {
		mux.Handle(fmt.Sprintf("GET /%s/{path...}", prefix), middleware.CorrelationID(middleware.RequestLogger(fs)))
	}
}

func handle(h handler.HandlerMethod) http.Handler {
	return middleware.CorrelationID(middleware.RequestLogger(route_handler.RouteHandler(h)))
}
