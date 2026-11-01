package router

import (
	"fmt"
	"net/http"

	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/route_handler"
)

type Router interface {
	http.Handler
}

// auth is the Auth middleware wrapping every route, set by New as it refreshes
// the tokens through the handler.
var auth func(http.Handler) http.Handler

func New(h *handler.Handler) Router {
	auth = middleware.Auth(h.RefreshSession)
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handlePub(h.HomePage))

	mux.Handle("GET /merchant-spending", handle(h.MerchantSpending))
	mux.Handle("GET /dashboard/budgets", handle(h.DashboardBudgets))
	mux.Handle("GET /dashboard/categories", handle(h.DashboardCategories))
	mux.Handle("GET /dashboard/goals", handle(h.DashboardGoals))
	mux.Handle("GET /dashboard/activity", handle(h.DashboardActivity))
	mux.Handle("GET /dashboard/accounts", handle(h.DashboardAccounts))

	mux.Handle("GET /health", handlePubJSON(h.Health))

	mux.Handle("GET /accounts", handle(h.Accounts))
	mux.Handle("POST /accounts", handle(h.CreateAccount))
	mux.Handle("GET /accounts/fix-balances", handle(h.FixBalancesPage))
	mux.Handle("POST /accounts/fix-balances", handle(h.FixBalances))
	mux.Handle("GET /accounts/{id}", handle(h.Account))
	mux.Handle("GET /accounts/{id}/edit", handle(h.GetEditAccount))
	mux.Handle("PUT /accounts/{id}", handle(h.EditAccount))
	mux.Handle("PUT /accounts/{id}/default", handle(h.SetDefaultAccount))
	mux.Handle("PUT /accounts/{id}/archive", handle(h.ToggleArchiveAccount))
	mux.Handle("DELETE /accounts/{id}", handle(h.DeleteAccount))

	mux.Handle("GET /categories", handle(h.Categories))
	mux.Handle("POST /categories", handle(h.CreateCategory))
	mux.Handle("POST /categories/quick", handle(h.QuickCreateCategory))
	mux.Handle("GET /categories/{id}", handle(h.Category))
	mux.Handle("GET /categories/{id}/edit", handle(h.GetEditCategory))
	mux.Handle("PUT /categories/{id}", handle(h.EditCategory))
	mux.Handle("DELETE /categories/{id}", handle(h.DeleteCategory))

	mux.Handle("GET /transactions", handle(h.Transactions))
	mux.Handle("POST /transactions", handle(h.CreateTransaction))
	mux.Handle("GET /transactions/more", handle(h.TransactionsMore))
	mux.Handle("GET /transactions/{id}", handle(h.Transaction))
	mux.Handle("GET /transactions/{id}/edit", handle(h.GetEditTransaction))
	mux.Handle("PUT /transactions/{id}", handle(h.EditTransaction))
	mux.Handle("PUT /transactions/{id}/fulfill", handle(h.FulfillTransaction))
	mux.Handle("PUT /transactions/{id}/reviewed", handle(h.MarkTransactionReviewed))
	mux.Handle("PUT /transactions/reviewed", handle(h.MarkAllTransactionsReviewed))
	mux.Handle("GET /transactions/{id}/category", handle(h.GetReviewCategoryPicker))
	mux.Handle("PUT /transactions/{id}/category", handle(h.ReviewTransactionCategory))
	mux.Handle("GET /transactions/{id}/duplicate", handle(h.DuplicateTransaction))
	mux.Handle("DELETE /transactions/{id}", handle(h.DeleteTransaction))
	mux.Handle("GET /transactions/{id}/reimburses", handle(h.GetReimbursePicker))
	mux.Handle("PUT /transactions/{id}/reimburses", handle(h.LinkReimbursement))
	mux.Handle("DELETE /transactions/{id}/reimburses", handle(h.UnlinkReimbursement))

	mux.Handle("POST /recurrence-form/", handle(h.RecurrenceForm))
	mux.Handle("POST /recurrence-form/{id}", handle(h.RecurrenceForm))

	mux.Handle("GET /recurrent-tx", handle(h.RecurrentTxs))
	mux.Handle("GET /recurrent-tx/{id}", handle(h.RecurrentTx))
	mux.Handle("GET /recurrent-tx/{id}/edit", handle(h.GetEditRecurrentTx))
	mux.Handle("PUT /recurrent-tx/{id}", handle(h.EditRecurrentTx))
	mux.Handle("POST /recurrent-tx/{id}/generate-txs", handle(h.GenerateTransactions))
	mux.Handle("DELETE /recurrent-tx/{id}", handle(h.DeleteRecurrentTx))

	mux.Handle("GET /goals", handle(h.Goals))
	mux.Handle("POST /goals", handle(h.CreateGoal))
	mux.Handle("GET /goals/{id}", handle(h.Goal))
	mux.Handle("GET /goals/{id}/edit", handle(h.GetEditGoal))
	mux.Handle("GET /goals/{id}/progress-form", handle(h.GetGoalProgressForm))
	mux.Handle("POST /goals/{id}/progress", handle(h.CreateGoalProgress))
	mux.Handle("POST /goals/{id}/progress-inputs", handle(h.GoalProgressInputs))
	mux.Handle("PUT /goals/{id}", handle(h.EditGoal))
	mux.Handle("PUT /goals/{id}/fulfill", handle(h.FulfillGoal))
	mux.Handle("PUT /goals/{id}/unfulfill", handle(h.UnfulfillGoal))
	mux.Handle("DELETE /goals/{id}", handle(h.DeleteGoal))

	mux.Handle("GET /decide", handle(h.Decide))
	mux.Handle("GET /decide/afford", handle(h.AffordSection))
	mux.Handle("POST /decide/afford", handle(h.Afford))
	mux.Handle("POST /decide/plan-preview", handle(h.PlanPreview))
	mux.Handle("PUT /decide/work-settings", handle(h.EditWorkSettings))

	mux.Handle("GET /wishlist", handle(h.Wishlist))
	mux.Handle("POST /wishlist", handle(h.CreateWishlistItem))
	mux.Handle("GET /wishlist/{id}/edit", handle(h.GetEditWishlistItem))
	mux.Handle("PUT /wishlist/{id}", handle(h.EditWishlistItem))
	mux.Handle("PUT /wishlist/{id}/abandon", handle(h.AbandonWishlistItem))
	mux.Handle("PUT /wishlist/{id}/restore", handle(h.RestoreWishlistItem))
	mux.Handle("GET /wishlist/{id}/buy", handle(h.BuyWishlistItem))
	mux.Handle("DELETE /wishlist/{id}", handle(h.DeleteWishlistItem))

	mux.Handle("GET /counterparties", handle(h.Counterparties))
	mux.Handle("POST /counterparties", handle(h.CreateCounterparty))
	mux.Handle("POST /counterparties/quick", handle(h.QuickCreateCounterparty))
	mux.Handle("GET /counterparties/{id}", handle(h.Counterparty))
	mux.Handle("GET /counterparties/{id}/edit", handle(h.GetEditCounterparty))
	mux.Handle("PUT /counterparties/{id}", handle(h.EditCounterparty))
	mux.Handle("GET /counterparties/{id}/merge", handle(h.GetMergeCounterparty))
	mux.Handle("POST /counterparties/{id}/merge", handle(h.MergeCounterparty))
	mux.Handle("DELETE /counterparties/{id}", handle(h.DeleteCounterparty))

	mux.Handle("GET /currencies", handle(h.Currencies))
	mux.Handle("GET /currencies/{id}", handle(h.Currency))
	mux.Handle("PUT /currencies/{id}/show", handle(h.ChangeIsShownForCurrency(true)))
	mux.Handle("PUT /currencies/{id}/hide", handle(h.ChangeIsShownForCurrency(false)))
	mux.Handle("PUT /currencies/{id}/set-default", handle(h.SetDefaultCurrency))

	mux.Handle("POST /amount-inputs-with-currency/{id}", handle(h.AmountInputsWithCurrency))
	mux.Handle("POST /amount-inputs-with-account/{id}", handle(h.AmountInputsWithAccount))
	mux.Handle("POST /accounts-inputs/{id}", handle(h.AccountInputs))
	mux.Handle("POST /tx-inputs/{id}", handle(h.TxInputs))
	mux.Handle("POST /tx-inputs/switch/{id}", handle(h.TxInputsSwitch))

	mux.Handle("GET /oauth", handlePub(h.OAuth))
	mux.Handle("GET /oauth/cb", handlePub(h.OAuthCB))
	mux.Handle("GET /oauth/refresh-tokens", handlePub(h.RefreshTokens))
	mux.Handle("GET /oauth/mobile", handlePub(h.OAuthMobile))

	mux.Handle("POST /api/mobile/token", handlePubJSON(h.MobileToken))
	mux.Handle("POST /api/sms", handleDeviceJSON(h.IngestSMS, h.VerifyDevice))

	mux.Handle("GET /sms", handle(h.SMSPage))
	mux.Handle("POST /sms/retry", handle(h.RetryAllSMS))
	mux.Handle("POST /sms/ignore", handle(h.IgnoreAllSMS))
	mux.Handle("POST /sms/{id}/retry", handle(h.RetrySMS))
	mux.Handle("PUT /sms/{id}/ignore", handle(h.IgnoreSMS))
	mux.Handle("PUT /sms/timezone", handle(h.SetTimezone))

	mux.Handle("GET /sms/templates", handle(h.SMSTemplates))
	mux.Handle("POST /sms/templates", handle(h.CreateSMSTemplate))
	mux.Handle("GET /sms/templates/new", handle(h.NewSMSTemplate))
	mux.Handle("POST /sms/templates/sandbox", handle(h.SMSTemplateSandbox))
	mux.Handle("POST /sms/templates/preview", handle(h.PreviewSMSTemplate))
	mux.Handle("GET /sms/templates/{scope}/{id}/edit", handle(h.EditSMSTemplate))
	mux.Handle("PUT /sms/templates/{scope}/{id}", handle(h.UpdateSMSTemplate))
	mux.Handle("DELETE /sms/templates/{scope}/{id}", handle(h.DeleteSMSTemplate))

	mux.Handle("PUT /devices/{id}/revoke", handle(h.RevokeDevice))
	mux.Handle("GET /reset", handle(h.ResetPage))
	mux.Handle("POST /reset", handle(h.Reset))
	mux.Handle("GET /signout", handle(h.Signout))

	mux.Handle("GET /admin/fx-rates", handle(h.AdminFxRates, middleware.Admin))
	mux.Handle("GET /admin/fx-rates/status", handle(h.AdminFxRatesStatus, middleware.Admin))
	mux.Handle("POST /admin/fx-rates/refresh", handle(h.RefreshFxRates, middleware.Admin))

	static(mux, []string{
		"styles", "js", "images",
		"fonts", "icons", "favicons",
	}, "./web/public")

	brandIcons := http.StripPrefix("/brand-icons/", http.FileServerFS(brandicons.IconsFS()))
	mux.Handle("GET /brand-icons/{path...}", middleware.CorrelationID(middleware.RequestLogger(brandIcons)))

	mux.Handle("/", handlePub(h.NotFound))

	return mux
}

func static(mux *http.ServeMux, paths []string, fspath string) {
	fileServer := http.StripPrefix("/", http.FileServer(http.Dir(fspath)))
	// Revalidated on every load (a 304 when unchanged), as their URLs aren't
	// versioned and browsers would otherwise keep serving stale scripts.
	fs := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
	for _, prefix := range paths {
		mux.Handle(fmt.Sprintf("GET /%s/{path...}", prefix), middleware.CorrelationID(middleware.RequestLogger(fs)))
	}
}

func handle(h handler.HandlerMethod, appendMiddlewares ...(func(http.Handler) http.Handler)) http.Handler {
	appendMiddlewares = append(appendMiddlewares, middleware.Protected)
	return handlePub(h, appendMiddlewares...)
}

func handlePubJSON(h handler.HandlerJSONMethod, appendMiddlewares ...(func(http.Handler) http.Handler)) http.Handler {
	var handler http.Handler = route_handler.JSONRouteHandler(h)

	for i := len(appendMiddlewares) - 1; i >= 0; i-- {
		handler = appendMiddlewares[i](handler)
	}

	handler = auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}

func handleDeviceJSON(h handler.HandlerJSONMethod, verify middleware.DeviceVerifier) http.Handler {
	var handler http.Handler = route_handler.JSONRouteHandler(h)

	handler = middleware.DeviceAuth(verify)(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}

func handlePub(h handler.HandlerMethod, appendMiddlewares ...(func(http.Handler) http.Handler)) http.Handler {
	var handler http.Handler = route_handler.RouteHandler(h)

	for i := len(appendMiddlewares) - 1; i >= 0; i-- {
		handler = appendMiddlewares[i](handler)
	}

	handler = auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}
