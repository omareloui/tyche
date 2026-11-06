package handler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// Afford plans the purchase from the "Plan it" form.
func (h Handler) Afford(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	pi, err := h.planIt(r)
	if err != nil {
		return nil, err
	}
	return views.PlanItResult(pi.outcome(), pi.ws), nil
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

// AffordDrawer renders the "Plan it" drawer, prefilled from a wishlist item
// if asked (a wish cooling off planned for the day it's ready), with its
// result.
func (h Handler) AffordDrawer(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}
	claims := getClaims(r.Context())

	fd := &views.PlanItFD{AmountFD: &form.AmountFD{CurrencyID: form.FDInput{Value: claims.DefaultCurrency.ID}}}
	plan := &models.GoalPlan{}
	if id := r.URL.Query().Get("wishlist_id"); id != "" {
		item, err := h.repo.GetWishlistItem(r.Context(), id)
		if err != nil {
			return nil, err
		}
		fd.Name.Value, fd.WishlistID = item.Name, item.ID
		fd.AmountFD.Amount.Value = formatFloatIfNonZero(item.Price)
		fd.AmountFD.CurrencyID.Value = item.Currency.ID
		if item.Plan != nil {
			loan := *item.Plan
			plan.Mode, plan.Loan = models.GoalPlanInstallments, &loan
		}
	}

	r.Form = url.Values{"amount": {fd.AmountFD.Amount.Value}, "currency_id": {fd.AmountFD.CurrencyID.Value}, "name": {fd.Name.Value}}
	pi, err := h.planIt(r)
	if err != nil {
		return nil, err
	}
	plan.Monthly = pi.money.Free()
	if plan.Loan == nil {
		// A fifth down, to start from the usual bank loan.
		plan.Loan = &models.InstallmentPlan{Strategy: models.StrategyFlat, DownPayment: 20, DownPaymentPct: true}
	}
	pi.plan = plan
	fd.Plan = mapGoalPlanToFD(plan, nil)
	fd.Plan.Milestones = models.PlanMilestones(pi.goal, pi.ws, pi.ctx.Categories, pi.ctx.Goals, claims.MonthStart, time.Now())
	fd.Plan.Currency = pi.goal.Currency

	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}
	return views.AffordDrawer(fd, curs, &claims.DefaultCurrency, ws, pi.outcome()), nil
}

// planIt is a "Plan it" form read: the purchase as a goal starting from the
// money free now, its plan, and what it's weighed against.
type planIt struct {
	h       Handler
	r       *http.Request
	goal    *models.Goal
	plan    *models.GoalPlan
	money   models.GoalPlanMoney
	ctx     models.AffordContext
	ws      *models.WorkSettings
	problem string
}

// affordContext is what's weighed against the money: the balances, this
// month's budgets left, the goals saving or paying a loan off and the work
// settings.
func (h Handler) affordContext(ctx context.Context) (models.AffordContext, error) {
	claims := getClaims(ctx)
	balances, err := h.repo.GetBalances(ctx)
	if err != nil {
		return models.AffordContext{}, err
	}
	all, err := h.repo.GetGoals(ctx)
	if err != nil {
		return models.AffordContext{}, err
	}
	// The goals saving, and the ones still paying their loan off.
	goals := slicesutils.Filter(all, func(gl models.Goal) bool { return gl.FulfilledAt.IsZero() || gl.PayingOff() })
	paces, err := h.repo.GetGoalPaces(ctx, goals)
	if err != nil {
		return models.AffordContext{}, err
	}
	ws, err := h.repo.GetWorkSettings(ctx)
	if err != nil {
		return models.AffordContext{}, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		return models.AffordContext{}, err
	}
	bs, err := h.repo.GetBudgetSummary(ctx, time.Time{}, &claims.DefaultCurrency, nil)
	if err != nil {
		return models.AffordContext{}, err
	}
	return models.AffordContext{
		Balances: balances, BudgetLeft: bs.Unspent(), Categories: cats,
		Goals: goals, Paces: paces, Work: ws,
		Start: claims.MonthStart, Today: h.userToday(ctx), Currency: &claims.DefaultCurrency,
	}, nil
}


func (h Handler) planIt(r *http.Request) (*planIt, error) {
	ctx := r.Context()
	claims := getClaims(ctx)
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	ac, err := h.affordContext(ctx)
	if err != nil {
		return nil, err
	}
	ws, cats, goals := ac.Work, ac.Categories, ac.Goals
	pi := &planIt{h: h, r: r, ws: ws, ctx: ac}

	price, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		pi.problem = "Check the price."
		price = 0
	}
	cur := claims.DefaultCurrency
	if c := slicesutils.Find(curs, func(c models.Currency) bool { return c.ID == r.FormValue("currency_id") }); c != nil {
		cur = *c
	}
	// It starts from the money free now, as a goal would from its savings.
	free, _ := models.Convert(pi.ctx.Spendable(), &claims.DefaultCurrency, cur.String)
	free = math.Max(0, free)
	pi.goal = &models.Goal{
		Name: r.FormValue("name"), Price: price, Currency: cur,
		Account: &models.Account{Amount: free, Currency: cur, Kind: models.AccountKind{Enum: models.AccKindGoal}},
	}
	if pi.plan, err = mapFDToGoalPlan(r); err != nil && pi.problem == "" {
		pi.problem = "Check the numbers, some aren't valid."
	}
	pi.money = models.NewGoalPlanMoney(pi.goal, ws, cats, goals, claims.MonthStart, time.Now(), pi.plan.StartsOn)
	if pi.problem == "" && price <= 0 {
		pi.problem = "Enter a price to plan it."
	}
	return pi, nil
}

// outcome is the plan's options and the picked one weighed on the day it's
// bought: the month its target is saved, today when it is already. A
// problem leaves the options empty, told as the hint.
func (pi *planIt) outcome() views.PlanItOutcome {
	out := views.PlanItOutcome{Goal: pi.goal, Money: pi.money, Hint: pi.problem}
	if pi.problem != "" {
		if pi.plan != nil {
			out.Opts.Mode = pi.plan.Mode
		}
		return out
	}
	out.Opts = pi.h.goalPlanOptions(pi.goal, pi.plan)
	if pr := out.Opts.Picked(); pr != nil {
		p := models.Purchase{Price: pi.goal.Price, Currency: pi.goal.Currency}
		if pr.Loan != nil {
			loan := pr.Loan.Plan
			p.Plan = &loan
		}
		if pr.Left > 0.005 {
			p.On = pr.BuyOn
		}
		if res, err := models.CheckAffordability(p, pi.ctx); err == nil {
			out.Afford = &res
		}
	}
	return out
}

// saveGoalProblem is a "Save as a goal" problem, shown by the button.
func saveGoalProblem(msg string) error {
	return resperr.New(http.StatusUnprocessableEntity, msg)
}

// SaveAffordGoal saves the planned purchase as a goal with its plan (for the
// wish it's planned for, if any), opening its details.
func (h Handler) SaveAffordGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	pi, err := h.planIt(r)
	if err != nil {
		return nil, err
	}
	if pi.problem != "" {
		return nil, saveGoalProblem(pi.problem)
	}
	if strings.TrimSpace(pi.goal.Name) == "" {
		return nil, saveGoalProblem("Name it to save it as a goal.")
	}

	gl := &models.Goal{ID: idgen.New(), Name: pi.goal.Name, Price: pi.goal.Price, Currency: models.Currency{ID: pi.goal.Currency.ID}, CreatedAt: time.Now()}
	plan := pi.plan
	// A new goal starts with nothing saved.
	fd := mapGoalPlanToFD(plan, nil)
	switch h.checkGoalPlan(r, plan, nil, gl.Price, 0, fd) {
	case "":
	case planLoanProblem:
		return nil, saveGoalProblem(planProblem(goalLoanProblems(plan.Loan, gl.Price)))
	default:
		for _, msg := range []string{fd.StartsOn.Error, fd.Monthly.Error, fd.Months.Error} {
			if msg != "" {
				return nil, saveGoalProblem(msg + ".")
			}
		}
	}
	if plan.Mode == models.GoalPlanSave {
		plan.Loan = nil
	}

	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			return nil, saveGoalProblem("Check the name and the price.")
		}
		return nil, err
	}
	wishID := r.FormValue("wishlist_id")
	if wishID != "" {
		if item, err := h.repo.GetWishlistItem(r.Context(), wishID); err != nil || item.Status != models.WishlistWaiting {
			wishID = ""
		}
	}
	if wishID != "" {
		err = h.repo.CreateGoalFromWish(r.Context(), gl, wishID)
	} else {
		err = h.repo.CreateGoal(r.Context(), gl)
	}
	if err != nil {
		var valerr *errs.ValidationError
		if errors.As(err, &valerr) {
			return nil, saveGoalProblem("A goal with that name exists already.")
		}
		return nil, err
	}
	if err := h.repo.SetGoalPlan(r.Context(), gl.ID, plan); err != nil {
		return nil, err
	}
	l.Info("saved a plan as a goal", zap.String("goal", gl.ID), zap.String("wish", wishID))

	hxRespondWithRedirect(w, "/goals?"+views.GoalDetailsParam+"="+gl.ID)
	return nil, nil
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
