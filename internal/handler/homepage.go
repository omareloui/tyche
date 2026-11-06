package handler

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// How far ahead the dashboard lists scheduled transactions, and how many
// of the latest ones it shows.
const (
	upcomingDays = 7
	recentTxs    = 6
)

// HomePage is the dashboard once signed in. It only loads what's cheap, the
// other widgets are fetched by the page (Dashboard* handlers).
func (h Handler) HomePage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	claims := getClaims(ctx)

	if claims == nil {
		return views.HomePage(claims, []models.Balance{}), nil
	}

	balances, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	flow, err := h.repo.GetFlow(ctx, &claims.DefaultCurrency)
	if err != nil {
		return nil, err
	}
	review, err := h.repo.CountTransactionsNeedingReview(ctx)
	if err != nil {
		return nil, err
	}
	sms, err := h.repo.CountSMSNeedingAttention(ctx)
	if err != nil {
		return nil, err
	}
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	cards, err := h.repo.GetCardStatements(ctx)
	if err != nil {
		return nil, err
	}

	var ratesAt time.Time
	if h.fxJob != nil {
		ratesAt = h.fxJob.Status().LastSuccessAt
	}
	return views.Dashboard(views.DashboardData{
		Claims:       claims,
		Balances:     balances,
		Flow:         flow,
		NeedsReview:  review,
		SMSAttention: sms,
		Currencies:   curs,
		RatesAt:      ratesAt,
		Cards:        cards,
	}), nil
}

func (h *Handler) DashboardBudgets(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	claims := getClaims(r.Context())
	b, err := h.repo.GetBudgets(r.Context(), &claims.DefaultCurrency)
	if err != nil {
		return nil, err
	}
	return views.DashBudgets(b), nil
}

// DashboardSafeToSpend is what can be spent a day until the end of the
// user's month once its budgets, goal deposits and unbudgeted bills are
// set aside.
func (h *Handler) DashboardSafeToSpend(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	claims := getClaims(ctx)
	ac, err := h.affordContext(ctx)
	if err != nil {
		return nil, err
	}
	bills, err := h.repo.GetUnbudgetedScheduledOutflows(ctx, ac.Start.Of(ac.Today).To)
	if err != nil {
		return nil, err
	}
	return views.DashSafeToSpend(models.NewSafeToSpend(ac, bills), &claims.DefaultCurrency), nil
}

func (h *Handler) DashboardCategories(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	claims := getClaims(r.Context())

	// Zero for the current month.
	var month time.Time
	if v := r.URL.Query().Get("month"); v != "" {
		m, err := time.Parse("2006-01", v)
		if err != nil {
			return nil, resperr.New(http.StatusUnprocessableEntity, "invalid month")
		}
		month = m
	}

	cs, err := h.repo.GetCategorySpending(r.Context(), month, &claims.DefaultCurrency)
	if err != nil {
		return nil, err
	}
	return views.DashCategories(cs), nil
}

// DashboardNetWorth is the accounts summary's net worth over time, with the
// options picked on the accounts page in this browser.
func (h *Handler) DashboardNetWorth(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.repo.GetAccountsSummary(r.Context(), &getClaims(r.Context()).DefaultCurrency, accountsSummaryWith(r))
	if err != nil {
		return nil, err
	}
	return views.DashNetWorth(s), nil
}

// DashboardBudgetPace is this month's budgeted spending against spending the
// budgets evenly, without the categories hidden from the budget summary.
func (h *Handler) DashboardBudgetPace(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	_, hidden, err := summaryParams(r)
	if err != nil {
		return nil, err
	}
	bs, err := h.repo.GetBudgetSummary(r.Context(), time.Time{}, &getClaims(r.Context()).DefaultCurrency, hidden)
	if err != nil {
		return nil, err
	}
	return views.DashBudgetPace(bs), nil
}

func (h *Handler) DashboardGoals(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	goals, err := h.repo.GetUnfulfilledGoals(r.Context())
	if err != nil {
		return nil, err
	}
	return views.DashGoals(goals), nil
}

func (h *Handler) DashboardGlance(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	def := &getClaims(ctx).DefaultCurrency

	flow, err := h.repo.GetFlow(ctx, def)
	if err != nil {
		return nil, err
	}
	from := flow.Current().Period.From
	page, err := h.repo.GetTransactionsPage(ctx, models.TxFilter{
		Kind:   models.ExpenseTxKind,
		Status: models.TxStatusFulfilled,
		From:   from,
	}, time.Time{}, &from)
	if err != nil {
		return nil, err
	}
	return views.DashGlance(views.NewDashGlance(flow, page.History, def), def), nil
}

func (h *Handler) DashboardActivity(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	claims := getClaims(ctx)

	recent, err := h.repo.GetRecentTransactions(ctx, recentTxs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	until := time.Date(now.Year(), now.Month(), now.Day()+upcomingDays+1, 0, 0, 0, 0, time.UTC)
	upcoming, err := h.repo.GetUpcomingTransactions(ctx, until)
	if err != nil {
		return nil, err
	}
	return views.DashActivity(recent, upcoming, &claims.DefaultCurrency), nil
}

func (h *Handler) DashboardAccounts(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	claims := getClaims(ctx)

	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return views.DashAccounts(accs, &claims.DefaultCurrency), nil
}
