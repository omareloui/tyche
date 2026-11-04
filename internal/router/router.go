package router

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/route_handler"
)

type Router interface {
	http.Handler
}

// auth, deleted and vault are the Auth, Deleted and Vault middlewares
// wrapping every route, set by New as they go through the handler
// (refreshing the tokens, signing out deleted accounts, unlocking the
// encrypted databases).
var auth, deleted, vault func(http.Handler) http.Handler

// badge counts the installed apps' badge again after the requests of a
// tenant with one (middleware.Badge), set by New.
var badge func(http.Handler) http.Handler

func New(h *handler.Handler) Router {
	auth = middleware.Auth(h.RefreshSession)
	deleted = middleware.Deleted(h.IsTenantDeleted, handler.SignOutDeleted)
	vault = middleware.Vault(h.OpenVault)
	badge = middleware.Badge(h.RefreshBadge)
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handlePub(h.HomePage))

	mux.Handle("GET /merchant-spending", handle(h.MerchantSpending))
	mux.Handle("GET /dashboard/budgets", handle(h.DashboardBudgets))
	mux.Handle("GET /dashboard/categories", handle(h.DashboardCategories))
	mux.Handle("GET /dashboard/goals", handle(h.DashboardGoals))
	mux.Handle("GET /dashboard/glance", handle(h.DashboardGlance))
	mux.Handle("GET /dashboard/activity", handle(h.DashboardActivity))
	mux.Handle("GET /dashboard/accounts", handle(h.DashboardAccounts))

	mux.Handle("GET /health", handlePubJSON(h.Health))

	mux.Handle("GET /accounts", handle(h.Accounts))
	mux.Handle("POST /accounts", handle(h.CreateAccount))
	mux.Handle("GET /accounts/fix-balances", handle(h.FixBalancesPage))
	mux.Handle("POST /accounts/fix-balances", handle(h.FixBalances))
	mux.Handle("GET /accounts/{id}", handle(h.LiveAccount(h.Account)))
	mux.Handle("GET /accounts/{id}/details", handle(h.LiveAccount(h.AccountDetails)))
	mux.Handle("GET /accounts/{id}/edit", handle(h.LiveAccount(h.GetEditAccount)))
	mux.Handle("PUT /accounts/{id}", handle(h.LiveAccount(h.EditAccount)))
	mux.Handle("PUT /accounts/{id}/default", handle(h.LiveAccount(h.SetDefaultAccount)))
	mux.Handle("PUT /accounts/{id}/archive", handle(h.LiveAccount(h.ToggleArchiveAccount)))
	mux.Handle("GET /accounts/{id}/delete-form", handle(h.LiveAccount(h.GetAccountDeleteForm)))
	mux.Handle("POST /accounts/{id}/delete-inputs", handle(h.LiveAccount(h.AccountDeleteInputs)))
	mux.Handle("DELETE /accounts/{id}", handle(h.LiveAccount(h.DeleteAccount)))
	// With the delete drawer's form (htmx sends a DELETE's in its body, unparsed).
	mux.Handle("POST /accounts/{id}/delete", handle(h.LiveAccount(h.DeleteAccount)))

	mux.Handle("GET /categories", handle(h.Categories))
	mux.Handle("POST /categories", handle(h.CreateCategory))
	mux.Handle("POST /categories/quick", handle(h.QuickCreateCategory))
	mux.Handle("GET /categories/budget", handle(h.CategoriesBudget))
	mux.Handle("GET /categories/{id}", handle(h.Category))
	mux.Handle("GET /categories/{id}/edit", handle(h.GetEditCategory))
	mux.Handle("PUT /categories/{id}", handle(h.EditCategory))
	mux.Handle("DELETE /categories/{id}", handle(h.DeleteCategory))

	mux.Handle("GET /transactions", handle(h.Transactions))
	mux.Handle("POST /transactions", handle(h.CreateTransaction))
	mux.Handle("GET /transactions/more", handle(h.TransactionsMore))
	mux.Handle("GET /transactions/{id}/details", handle(h.TransactionDetails))
	mux.Handle("GET /transactions/{id}/edit", handle(h.GetEditTransaction))
	mux.Handle("PUT /transactions/{id}", handle(h.EditTransaction))
	mux.Handle("PUT /transactions/{id}/fulfill", handle(h.FulfillTransaction))
	mux.Handle("PUT /transactions/{id}/reviewed", handle(h.MarkTransactionReviewed))
	mux.Handle("PUT /transactions/reviewed", handle(h.MarkAllTransactionsReviewed))
	mux.Handle("GET /transactions/{id}/category", handle(h.GetReviewCategoryPicker))
	mux.Handle("PUT /transactions/{id}/category", handle(h.ReviewTransactionCategory))
	mux.Handle("GET /transactions/{id}/duplicate", handle(h.DuplicateTransaction))
	mux.Handle("DELETE /transactions/{id}", handle(h.DeleteTransaction))
	mux.Handle("POST /transactions/delete", handle(h.DeleteTransactions))
	mux.Handle("GET /transactions/{id}/reimburses", handle(h.GetReimbursePicker))
	mux.Handle("PUT /transactions/{id}/reimburses", handle(h.LinkReimbursement))
	mux.Handle("DELETE /transactions/{id}/reimburses", handle(h.UnlinkReimbursement))
	mux.Handle("GET /transactions/{id}/transfer-pair", handle(h.GetTransferPairPicker))
	mux.Handle("PUT /transactions/{id}/transfer-pair", handle(h.LinkTransferPair))
	mux.Handle("DELETE /transactions/{id}/transfer-pair", handle(h.UnlinkTransferPair))

	mux.Handle("POST /recurrence-form/", handle(h.RecurrenceForm))
	mux.Handle("POST /recurrence-form/{id}", handle(h.RecurrenceForm))

	mux.Handle("GET /recurrent-tx", handle(h.RecurrentTxs))
	mux.Handle("POST /recurrent-tx", handle(h.CreateRecurrentTx))
	mux.Handle("GET /recurrent-tx/{id}", handle(h.RecurrentTx))
	mux.Handle("GET /recurrent-tx/{id}/details", handle(h.RecurrentTxDetails))
	mux.Handle("GET /recurrent-tx/{id}/edit", handle(h.GetEditRecurrentTx))
	mux.Handle("PUT /recurrent-tx/{id}", handle(h.EditRecurrentTx))
	mux.Handle("POST /recurrent-tx/{id}/generate-txs", handle(h.GenerateTransactions))
	mux.Handle("DELETE /recurrent-tx/{id}", handle(h.DeleteRecurrentTx))

	mux.Handle("GET /goals", handle(h.Goals))
	mux.Handle("POST /goals", handle(h.CreateGoal))
	mux.Handle("GET /goals/{id}", handle(h.Goal))
	mux.Handle("GET /goals/{id}/details", handle(h.GoalDetails))
	mux.Handle("GET /goals/{id}/edit", handle(h.GetEditGoal))
	mux.Handle("GET /goals/{id}/money-form", handle(h.GetGoalMoneyForm))
	mux.Handle("POST /goals/{id}/money", handle(h.MoveGoalMoney))
	mux.Handle("POST /goals/{id}/money-inputs", handle(h.GoalMoneyInputs))
	mux.Handle("PUT /goals/{id}", handle(h.EditGoal))
	mux.Handle("GET /goals/{id}/fulfill-form", handle(h.GetGoalFulfillForm))
	mux.Handle("PUT /goals/{id}/fulfill", handle(h.FulfillGoal))
	mux.Handle("GET /goals/{id}/delete-form", handle(h.GetGoalDeleteForm))
	mux.Handle("POST /goals/{id}/delete", handle(h.DeleteGoal))
	mux.Handle("PUT /goals/{id}/unfulfill", handle(h.UnfulfillGoal))
	mux.Handle("GET /goals/{id}/plan", handle(h.GetGoalPlanForm))
	mux.Handle("POST /goals/{id}/plan/preview", handle(h.GoalPlanPreview))
	mux.Handle("PUT /goals/{id}/plan", handle(h.SetGoalPlan))
	mux.Handle("DELETE /goals/{id}/plan", handle(h.DeleteGoalPlan))
	mux.Handle("DELETE /goals/{id}", handle(h.DeleteGoal))

	mux.Handle("GET /decide", handle(h.Decide))
	mux.Handle("GET /decide/afford", handle(h.AffordSection))
	mux.Handle("POST /decide/afford", handle(h.Afford))
	mux.Handle("POST /decide/plan-preview", handle(h.PlanPreview))

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
	mux.Handle("GET /counterparties/{id}/details", handle(h.CounterpartyDetails))
	mux.Handle("GET /counterparties/{id}/edit", handle(h.GetEditCounterparty))
	mux.Handle("PUT /counterparties/{id}", handle(h.EditCounterparty))
	mux.Handle("PUT /counterparties/{id}/archive", handle(h.ToggleArchiveCounterparty))
	mux.Handle("PUT /counterparties/{id}/rating", handle(h.RateCounterparty))
	mux.Handle("POST /counterparties/{id}/locations", handle(h.AddCounterpartyLocation))
	mux.Handle("PUT /counterparties/{id}/labels", handle(h.SetCounterpartyLabels))
	mux.Handle("GET /counterparties/{id}/merge", handle(h.GetMergeCounterparty))
	mux.Handle("POST /counterparties/{id}/merge", handle(h.MergeCounterparty))
	mux.Handle("DELETE /counterparties/{id}", handle(h.DeleteCounterparty))

	mux.Handle("GET /currencies", handle(h.Currencies))
	mux.Handle("GET /currencies/{id}", handle(h.Currency))
	mux.Handle("PUT /currencies/{id}/show", handle(h.ChangeIsShownForCurrency(true)))
	mux.Handle("PUT /currencies/{id}/hide", handle(h.ChangeIsShownForCurrency(false)))
	mux.Handle("PUT /currencies/{id}/set-default", handle(h.SetDefaultCurrency))
	mux.Handle("PUT /currencies/{id}/unit", handle(h.SetCurrencyUnit))

	mux.Handle("POST /amount-inputs-with-currency/{id}", handle(h.AmountInputsWithCurrency))
	mux.Handle("POST /amount-inputs-with-account/{id}", handle(h.AmountInputsWithAccount))
	mux.Handle("POST /accounts-inputs/{id}", handle(h.AccountInputs))
	mux.Handle("POST /tx-inputs/{id}", handle(h.TxInputs))
	mux.Handle("POST /tx-inputs/switch/{id}", handle(h.TxInputsSwitch))

	mux.Handle("GET /oauth", handlePub(h.OAuth))
	mux.Handle("GET /oauth/cb", handlePub(h.OAuthCB))
	mux.Handle("POST /oauth/finish", handlePub(h.OAuthFinish))
	mux.Handle("POST /oauth/setup", handlePub(h.OAuthSetup))
	mux.Handle("GET /oauth/refresh-tokens", handlePub(h.RefreshTokens))
	mux.Handle("GET /oauth/mobile", handlePub(h.OAuthMobile))

	mux.Handle("POST /api/mobile/token", handlePubJSON(h.MobileToken))
	mux.Handle("POST /api/sms", handleDeviceJSON(h.IngestSMS, h.VerifyDevice))

	mux.Handle("GET /sms", handle(h.SMSPage))
	mux.Handle("POST /sms/retry", handle(h.RetryAllSMS))
	mux.Handle("POST /sms/ignore", handle(h.IgnoreAllSMS))
	mux.Handle("POST /sms/pair-transfers", handle(h.PairTransfers))
	mux.Handle("POST /sms/{id}/retry", handle(h.RetrySMS))
	mux.Handle("PUT /sms/{id}/ignore", handle(h.IgnoreSMS))

	mux.Handle("GET /sms/templates", handle(h.SMSTemplates))
	mux.Handle("POST /sms/templates", handle(h.CreateSMSTemplate))
	mux.Handle("GET /sms/templates/new", handle(h.NewSMSTemplate))
	mux.Handle("POST /sms/templates/sandbox", handle(h.SMSTemplateSandbox))
	mux.Handle("POST /sms/templates/preview", handle(h.PreviewSMSTemplate))
	mux.Handle("GET /sms/templates/{scope}/{id}/edit", handle(h.EditSMSTemplate))
	mux.Handle("PUT /sms/templates/{scope}/{id}", handle(h.UpdateSMSTemplate))
	mux.Handle("DELETE /sms/templates/{scope}/{id}", handle(h.DeleteSMSTemplate))

	mux.Handle("GET /welcome", handle(h.Welcome))
	mux.Handle("POST /welcome/region", handle(h.WelcomeRegion, middleware.HTMXOnly))
	mux.Handle("POST /welcome/accounts", handle(h.WelcomeAddAccount, middleware.HTMXOnly))
	mux.Handle("POST /welcome/balances", handle(h.WelcomeBalances, middleware.HTMXOnly))
	mux.Handle("POST /welcome/finish", handle(h.FinishWelcome, middleware.HTMXOnly))

	mux.Handle("GET /unlock", handlePub(h.UnlockPage))
	mux.Handle("POST /unlock", handlePub(h.Unlock))
	mux.Handle("POST /unlock/passkey/options", handlePubJSON(h.UnlockPasskeyOptions, middleware.HTMXOnly))
	mux.Handle("POST /unlock/passkey", handlePubJSON(h.UnlockPasskey, middleware.HTMXOnly))
	mux.Handle("GET /settings", handle(h.Settings))
	mux.Handle("GET /badge", handlePubJSON(h.BadgeCounts, middleware.Protected))
	mux.Handle("GET /push/key", handlePubJSON(h.PushKey, middleware.Protected))
	mux.Handle("PUT /push/subscription", handlePubJSON(h.Subscribe, middleware.Protected, middleware.HTMXOnly))
	mux.Handle("DELETE /push/subscription", handlePubJSON(h.Unsubscribe, middleware.Protected, middleware.HTMXOnly))
	mux.Handle("PUT /settings/profile", handle(h.SetProfile, middleware.HTMXOnly))
	mux.Handle("PUT /settings/region", handle(h.SetRegionSettings, middleware.HTMXOnly))
	mux.Handle("PUT /settings/month", handle(h.SetMonthStart, middleware.HTMXOnly))
	mux.Handle("PUT /settings/work", handle(h.SetWorkSettings, middleware.HTMXOnly))
	mux.Handle("PUT /settings/account", handle(h.SetDefaultAccountSettings, middleware.HTMXOnly))
	mux.Handle("GET /security", handle(h.Security))
	mux.Handle("POST /security/encryption", handle(h.EnableEncryption, middleware.HTMXOnly))
	mux.Handle("POST /security/encryption/disable", handle(h.DisableEncryption, middleware.HTMXOnly))
	mux.Handle("POST /security/password", handle(h.ChangeVaultPassword, middleware.HTMXOnly))
	mux.Handle("POST /security/recovery-code", handle(h.RegenerateRecoveryCode, middleware.HTMXOnly))
	mux.Handle("POST /security/sessions/lock-others", handle(h.LockOtherSessions, middleware.HTMXOnly))
	mux.Handle("POST /security/lock", handle(h.Lock, middleware.HTMXOnly))
	mux.Handle("POST /security/passkeys/options", handlePubJSON(h.PasskeyOptions, middleware.Protected, middleware.HTMXOnly))
	mux.Handle("POST /security/passkeys", handle(h.AddPasskey, middleware.HTMXOnly))
	mux.Handle("POST /security/passkeys/{id}/delete", handle(h.DeletePasskey, middleware.HTMXOnly))

	mux.Handle("PUT /devices/{id}/revoke", handle(h.RevokeDevice))
	mux.Handle("GET /reset", handle(h.ResetPage))
	mux.Handle("POST /reset", handle(h.Reset))

	mux.Handle("GET /backup", handle(h.BackupPage))
	mux.Handle("GET /backup/export", handle(h.ExportBackup))
	mux.Handle("POST /backup/import", handle(h.ImportBackup, middleware.HTMXOnly))
	mux.Handle("POST /backup/restore", handle(h.RestorePrevBackup, middleware.HTMXOnly))

	// Public: signing out of a locked database has no access token.
	mux.Handle("GET /signout", handlePub(h.Signout))
	mux.Handle("GET /avatar", handle(h.Avatar))

	mux.Handle("GET /admin/fx-rates", handle(h.AdminFxRates, middleware.Admin))
	mux.Handle("GET /admin/fx-rates/status", handle(h.AdminFxRatesStatus, middleware.Admin))
	mux.Handle("POST /admin/fx-rates/refresh", handle(h.RefreshFxRates, middleware.Admin))
	mux.Handle("GET /admin/tenants", handle(h.AdminTenants, middleware.Admin))
	mux.Handle("POST /admin/tenants/{id}/delete", handle(h.DeleteTenant, middleware.Admin))

	static(mux, []string{
		"styles", "js", "images",
		"fonts", "icons", "favicons",
	}, "./web/public")
	// The service worker's scope is its own path, so it and the manifest are
	// served from the root.
	rootFile(mux, "sw.js", "text/javascript; charset=utf-8", "./web/public")
	rootFile(mux, "manifest.webmanifest", "application/manifest+json", "./web/public")
	// Cached by the service worker for everyone: no auth/vault middlewares,
	// which could answer it with a redirect to sign in or unlock.
	mux.Handle("GET /offline", middleware.CorrelationID(middleware.RequestLogger(route_handler.RouteHandler(h.Offline))))

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

// rootFile serves fspath/name at /name, revalidated like static.
func rootFile(mux *http.ServeMux, name, contentType, fspath string) {
	path := filepath.Join(fspath, name)
	fs := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)
	})
	mux.Handle("GET /"+name, middleware.CorrelationID(middleware.RequestLogger(fs)))
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

	handler = badge(handler)
	handler = vault(handler)
	handler = deleted(handler)
	handler = auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}

func handleDeviceJSON(h handler.HandlerJSONMethod, verify middleware.DeviceVerifier) http.Handler {
	var handler http.Handler = route_handler.JSONRouteHandler(h)

	handler = badge(handler)
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

	handler = badge(handler)
	handler = vault(handler)
	handler = deleted(handler)
	handler = auth(handler)
	handler = middleware.RequestLogger(handler)
	handler = middleware.CorrelationID(handler)

	return handler
}
