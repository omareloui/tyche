package handler

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
)

// Afford checks a purchase from the afford form.
func (h Handler) Afford(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	p, err := mapFDToPurchase(r, curs, &claims.DefaultCurrency)
	if err != nil {
		return views.AffordHint("Check the numbers, some aren't valid."), nil
	}
	if p.Price <= 0 {
		return views.AffordResult(nil, nil), nil
	}
	if p.Plan != nil {
		if ve := p.Plan.Validate(p.Price); len(ve) > 0 {
			return views.AffordHint(planProblem(ve)), nil
		}
	}

	return h.affordResult(r, p)
}

// PlanPreview sums up the plan being filled in a form.
func (h Handler) PlanPreview(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	p, err := mapFDToPurchase(r, curs, &claims.DefaultCurrency)
	if err != nil {
		return views.PlanPreview(nil, "", "Check the numbers, some aren't valid."), nil
	}
	if p.Plan == nil || p.Price <= 0 {
		return views.PlanPreview(nil, "", ""), nil
	}
	if ve := p.Plan.Validate(p.Price); len(ve) > 0 {
		return views.PlanPreview(nil, "", planProblem(ve)), nil
	}

	s, err := p.Schedule()
	if err != nil {
		return views.PlanPreview(nil, "", err.Error()), nil
	}
	return views.PlanPreview(s, p.Currency.Symbol, ""), nil
}

// planFieldLabels name a plan's fields in its problems.
var planFieldLabels = map[string]string{
	"Installments":      "Months",
	"InstallmentAmount": "Per month",
	"Total":             "Total",
	"Rate":              "Interest",
	"DownPayment":       "Down payment",
	"AdminFee":          "Admin fee",
	"MonthlyFee":        "Monthly fee",
	"InsuranceRate":     "Insurance",
	"BalloonPct":        "Last payment",
	"GraceMonths":       "Grace months",
}

// planProblem is the first of a plan's problems, as text.
func planProblem(ve errs.ValidationErrors) string {
	for _, key := range []string{"Installments", "InstallmentAmount", "Total", "Rate", "DownPayment", "AdminFee", "MonthlyFee", "InsuranceRate", "BalloonPct", "GraceMonths"} {
		if msg := ve.MsgFor(key); msg != "" {
			return fmt.Sprintf("%s: %s.", planFieldLabels[key], msg)
		}
	}
	return ""
}

// AffordDrawer renders the afford drawer, prefilled from a wishlist item if
// asked, with its result.
func (h Handler) AffordDrawer(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	p := &models.Purchase{Currency: claims.DefaultCurrency}
	name := ""

	if id := r.URL.Query().Get("wishlist_id"); id != "" {
		item, err := h.repo.GetWishlistItem(r.Context(), id)
		if err != nil {
			return nil, err
		}
		name = item.Name
		purchase := item.Purchase()
		p = &purchase
		// A wish still cooling off is checked on the day it's ready.
		y, m, d := item.ReadyAt().Date()
		if ready := time.Date(y, m, d, 0, 0, 0, 0, time.UTC); item.Status == models.WishlistWaiting && ready.After(h.userToday(r.Context())) {
			p.On = ready
		}
	}

	result, err := h.affordResult(r, p)
	if err != nil {
		return nil, err
	}
	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}
	fd := mapPurchaseToFD(p, nil)
	fd.On = &form.FDInput{}
	if !p.On.IsZero() {
		fd.On.Value = p.On.Format(time.DateOnly)
	}
	return views.AffordDrawer(fd, name, curs, &claims.DefaultCurrency, ws, result), nil
}

func (h Handler) affordResult(r *http.Request, p *models.Purchase) (templ.Component, error) {
	if p.Price <= 0 {
		return views.AffordResult(nil, nil), nil
	}
	ctx := r.Context()
	claims := getClaims(ctx)

	balances, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	all, err := h.repo.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	// The goals saving, and the ones still paying their loan off.
	goals := slicesutils.Filter(all, func(gl models.Goal) bool { return gl.FulfilledAt.IsZero() || gl.PayingOff() })
	paces, err := h.repo.GetGoalPaces(ctx, goals)
	if err != nil {
		return nil, err
	}
	ws, err := h.repo.GetWorkSettings(ctx)
	if err != nil {
		return nil, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	bs, err := h.repo.GetBudgetSummary(ctx, time.Time{}, &claims.DefaultCurrency, nil)
	if err != nil {
		return nil, err
	}

	a, err := models.CheckAffordability(*p, models.AffordContext{
		Balances: balances, BudgetLeft: bs.Unspent(), Categories: cats,
		Goals: goals, Paces: paces, Work: ws,
		Start: claims.MonthStart, Today: h.userToday(ctx), Currency: &claims.DefaultCurrency,
	})
	if err != nil {
		return views.AffordHint(err.Error()), nil
	}
	return views.AffordResult(&a, ws), nil
}

// mapPurchaseToFD fills the purchase's form, with the plan's problems if
// any.
func mapPurchaseToFD(p *models.Purchase, ve errs.ValidationErrors) *views.PurchaseFD {
	fd := &views.PurchaseFD{
		AmountFD: &form.AmountFD{
			Amount:     form.FDInput{Value: formatFloatIfNonZero(p.Price), Error: ve.MsgFor("Price")},
			CurrencyID: form.FDInput{Value: p.Currency.ID, Error: ve.MsgFor("CurrencyID")},
		},
		Mode: form.FDInput{Value: views.PayCash},
	}

	plan := p.Plan
	if plan == nil {
		plan = &models.InstallmentPlan{}
	} else {
		fd.Mode.Value = views.PayInstallments
	}

	fd.Plan = mapPlanToFD(plan, ve)
	if planProblem(ve) != "" {
		fd.Mode.Value = views.PayInstallments
	}
	return fd
}

// mapPlanToFD fills an installment plan's fields, with its problems if any.
func mapPlanToFD(plan *models.InstallmentPlan, ve errs.ValidationErrors) views.InstallmentPlanFD {
	unit := func(pct bool) form.FDInput {
		if pct {
			return form.FDInput{Value: views.UnitPct}
		}
		return form.FDInput{Value: views.UnitAmount}
	}
	// A known amount plan with both its months and amount was given a total.
	termBy, total := views.TermByMonths, 0.0
	switch {
	case plan.Months > 0 && plan.Amount > 0:
		total = models.RoundCents(float64(plan.Months) * plan.Amount)
	case plan.Amount > 0:
		termBy = views.TermByAmount
	}
	return views.InstallmentPlanFD{
		Strategy:          form.FDInput{Value: plan.Strategy.String()},
		Installments:      form.FDInput{Value: formatIntIfNonZero(plan.Months), Error: ve.MsgFor("Installments")},
		InstallmentAmount: form.FDInput{Value: formatFloatIfNonZero(plan.Amount), Error: ve.MsgFor("InstallmentAmount")},
		TermBy:            form.FDInput{Value: termBy},
		Total:             form.FDInput{Value: formatFloatIfNonZero(total), Error: ve.MsgFor("Total")},
		Rate:              form.FDInput{Value: formatFloatIfNonZero(plan.Rate), Error: ve.MsgFor("Rate")},
		RatePeriod:        form.FDInput{Value: plan.RatePeriod.String()},
		DownPayment:       form.FDInput{Value: formatFloatIfNonZero(plan.DownPayment), Error: ve.MsgFor("DownPayment")},
		DownPaymentUnit:   unit(plan.DownPaymentPct),
		AdminFee:          form.FDInput{Value: formatFloatIfNonZero(plan.AdminFee), Error: ve.MsgFor("AdminFee")},
		AdminFeeUnit:      unit(plan.AdminFeePct),
		AdminFeeFinanced:  form.FDInput{Value: formatBooleanIfNonZero(plan.AdminFeeFinanced)},
		MonthlyFee:        form.FDInput{Value: formatFloatIfNonZero(plan.MonthlyFee), Error: ve.MsgFor("MonthlyFee")},
		InsuranceRate:     form.FDInput{Value: formatFloatIfNonZero(plan.InsuranceRate), Error: ve.MsgFor("InsuranceRate")},
		BalloonPct:        form.FDInput{Value: formatFloatIfNonZero(plan.BalloonPct), Error: ve.MsgFor("BalloonPct")},
		GraceMonths:       form.FDInput{Value: formatIntIfNonZero(plan.GraceMonths), Error: ve.MsgFor("GraceMonths")},
	}
}

// mapFDToPurchase reads a price, its currency and, when paying in
// installments, the plan.
func mapFDToPurchase(r *http.Request, curs []models.Currency, dflt *models.Currency) (*models.Purchase, error) {
	price, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	on, err := parseDateOnlyIfExists(r.FormValue("on"))
	if err != nil {
		return nil, err
	}

	p := &models.Purchase{Price: price, Currency: *dflt, On: on}
	if cur := slicesutils.Find(curs, func(c models.Currency) bool { return c.ID == r.FormValue("currency_id") }); cur != nil {
		p.Currency = *cur
	}

	if r.FormValue("mode") == views.PayInstallments {
		plan, err := mapFDToInstallmentPlan(r)
		if err != nil {
			return nil, err
		}
		p.Plan = plan
	}

	return p, nil
}

func mapFDToInstallmentPlan(r *http.Request) (*models.InstallmentPlan, error) {
	var err error
	float := func(name string) float64 {
		v, e := parseFloatIfExists(r.FormValue(name))
		if e != nil {
			err = e
		}
		return v
	}
	integer := func(name string) int {
		v, e := parseIntIfExists(r.FormValue(name))
		if e != nil {
			err = e
		}
		return v
	}

	plan := &models.InstallmentPlan{
		Strategy:         models.InstallmentStrategyFromString(r.FormValue("strategy")),
		Months:           integer("installments"),
		Rate:             float("rate"),
		DownPayment:      float("down_payment"),
		DownPaymentPct:   r.FormValue("down_payment_unit") == views.UnitPct,
		AdminFee:         float("admin_fee"),
		AdminFeePct:      r.FormValue("admin_fee_unit") == views.UnitPct,
		AdminFeeFinanced: parseBoolean(r.FormValue("admin_fee_financed")),
		MonthlyFee:       float("monthly_fee"),
		InsuranceRate:    float("insurance_rate"),
		BalloonPct:       float("balloon_pct"),
		GraceMonths:      integer("grace_months"),
	}
	if r.FormValue("rate_period") == models.RatePerMonth.String() {
		plan.RatePeriod = models.RatePerMonth
	}
	// The amount and the rate only apply to their strategies.
	if plan.Strategy == models.StrategyKnownAmount {
		plan.Rate = 0
		termPlan(plan, r.FormValue("term_by"), float("installment_amount"), float("installments_total"))
	}
	return plan, err
}

// termPlan sets a known amount plan's term from what it's given by: its
// months (the amount worked out from the total, else from the price) or its
// amount per month (the months from the total, else from the price).
func termPlan(plan *models.InstallmentPlan, termBy string, amount, total float64) {
	if termBy == views.TermByAmount {
		plan.Months, plan.Amount = 0, amount
		if total > 0 && amount > 0 {
			plan.Months = int(math.Ceil(total/amount - 0.005))
		}
		return
	}
	plan.Amount = 0
	if total > 0 && plan.Months > 0 {
		plan.Amount = models.RoundCents(total / float64(plan.Months))
	}
}
