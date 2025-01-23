package router

import (
	"fmt"
	"net/http"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/route_handler"
)

type Router interface {
	http.Handler
}

func New(h *handler.Handler) Router {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handlePub(h.HomePage))

	mux.Handle("GET /accounts", handle(h.Accounts))
	mux.Handle("POST /accounts", handle(h.CreateAccount))
	mux.Handle("GET /accounts/{id}", handle(h.Account))
	mux.Handle("GET /accounts/{id}/edit", handle(h.GetEditAccount))
	mux.Handle("PUT /accounts/{id}", handle(h.EditAccount))
	mux.Handle("PUT /accounts/{id}/default", handle(h.SetDefaultAccount))
	mux.Handle("DELETE /accounts/{id}", handle(h.DeleteAccount))

	mux.Handle("GET /categories", handle(h.Categories))
	mux.Handle("POST /categories", handle(h.CreateCategory))
	mux.Handle("GET /categories/{id}", handle(h.Category))
	mux.Handle("GET /categories/{id}/edit", handle(h.GetEditCategory))
	mux.Handle("PUT /categories/{id}", handle(h.EditCategory))
	mux.Handle("DELETE /categories/{id}", handle(h.DeleteCategory))

	mux.Handle("GET /transactions", handle(h.Transactions))
	mux.Handle("POST /transactions", handle(h.CreateTransaction))
	mux.Handle("GET /transactions/{id}", handle(h.Transaction))
	mux.Handle("GET /transactions/{id}/edit", handle(h.GetEditTransaction))
	mux.Handle("PUT /transactions/{id}", handle(h.EditTransaction))
	mux.Handle("PUT /transactions/{id}/fulfill", handle(h.FulfillTransaction))
	mux.Handle("DELETE /transactions/{id}", handle(h.DeleteTransaction))

	mux.Handle("POST /frequency-form/", handle(h.FrequencyForm))
	mux.Handle("POST /frequency-form/{id}", handle(h.FrequencyForm))

	mux.Handle("GET /frequent-tx", handle(h.FrequentTxs))
	mux.Handle("GET /frequent-tx/{id}", handle(h.FrequentTx))
	mux.Handle("GET /frequent-tx/{id}/edit", handle(h.GetEditFrequentTx))
	mux.Handle("PUT /frequent-tx/{id}", handle(h.EditFrequentTx))
	mux.Handle("POST /frequent-tx/{id}/generate-txs", handle(h.GenerateTransactions))
	mux.Handle("DELETE /frequent-tx/{id}", handle(h.DeleteFrequentTx))

	mux.Handle("GET /goals", handle(h.Goals))
	mux.Handle("POST /goals", handle(h.CreateGoal))
	mux.Handle("GET /goals/{id}", handle(h.Goal))
	mux.Handle("GET /goals/{id}/edit", handle(h.GetEditGoal))
	mux.Handle("GET /goals/{id}/progress-form", handle(h.GetGoalProgressForm))
	mux.Handle("POST /goals/{id}/progress", handle(h.CreateGoalProgress))
	mux.Handle("PUT /goals/{id}/{$}", handle(h.EditGoal))
	mux.Handle("PUT /goals/{id}/fulfill", handle(h.FulfillGoal))
	mux.Handle("PUT /goals/{id}/unfulfill", handle(h.UnfulfillGoal))
	mux.Handle("DELETE /goals/{id}", handle(h.DeleteGoal))

	mux.Handle("GET /items", handle(h.Items))
	mux.Handle("POST /items", handle(h.CreateItem))
	mux.Handle("GET /items/{id}", handle(h.Item))
	mux.Handle("GET /items/{id}/edit", handle(h.GetEditItem))
	mux.Handle("PUT /items/{id}", handle(h.EditItem))
	mux.Handle("DELETE /items/{id}", handle(h.DeleteItem))

	mux.Handle("GET /counterparties", handle(h.Counterparties))
	mux.Handle("POST /counterparties", handle(h.CreateCounterparty))
	mux.Handle("GET /counterparties/{id}", handle(h.Counterparty))
	mux.Handle("GET /counterparties/{id}/edit", handle(h.GetEditCounterparty))
	mux.Handle("PUT /counterparties/{id}", handle(h.EditCounterparty))
	mux.Handle("DELETE /counterparties/{id}", handle(h.DeleteCounterparty))

	mux.Handle("POST /amount-inputs-with-currency/{id}", handle(h.AmountInputsWithCurrency))
	mux.Handle("POST /amount-inputs-with-account/{id}", handle(h.AmountInputsWithAccount))
	mux.Handle("POST /accounts-inputs/{id}", handle(h.AccountInputs))
	mux.Handle("POST /tx-inputs/{id}", handle(h.TxInputs))
	mux.Handle("POST /tx-inputs/switch/{id}", handle(h.TxInputsSwitch))

	mux.Handle("GET /oauth", handlePub(h.OAuth))
	mux.Handle("GET /oauth/cb", handlePub(h.OAuthCB))
	mux.Handle("GET /signout", handle(h.Signout))

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

func handle(h handler.HandlerMethod, appendMiddlewares ...(func(http.Handler) http.Handler)) http.Handler {
	var handler http.Handler = route_handler.RouteHandler(h)

	for i := len(appendMiddlewares) - 1; i >= 0; i-- {
		handler = appendMiddlewares[i](handler)
	}

	handler = middleware.Protected(handler)
	handler = middleware.Auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}

func handlePub(h handler.HandlerMethod, appendMiddlewares ...(func(http.Handler) http.Handler)) http.Handler {
	var handler http.Handler = route_handler.RouteHandler(h)

	for i := len(appendMiddlewares) - 1; i >= 0; i-- {
		handler = appendMiddlewares[i](handler)
	}

	handler = middleware.Auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}
