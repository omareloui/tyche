package handler

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// goalPlanDeps are a goal and the month's money free for its plan.
func (h Handler) goalPlanDeps(r *http.Request) (*models.Goal, models.GoalPlanMoney, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, models.GoalPlanMoney{}, errs.ErrInvalidID
	}
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, models.GoalPlanMoney{}, err
	}
	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, models.GoalPlanMoney{}, err
	}
	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, models.GoalPlanMoney{}, err
	}
	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		return nil, models.GoalPlanMoney{}, err
	}
	return gl, models.NewGoalPlanMoney(gl, ws, cats, gls, time.Now()), nil
}

// GetGoalPlanForm is the drawer planning how to save for the goal.
func (h Handler) GetGoalPlanForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	gl, money, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}

	plan := &models.GoalPlan{Monthly: money.Free()}
	if gl.Plan != nil {
		*plan = *gl.Plan
	} else if item, err := h.repo.GetWishlistItemByGoal(r.Context(), gl.ID); err != nil {
		return nil, err
	} else if item != nil && item.Plan != nil {
		// Saving for a wish paid in installments: its plan's loan.
		loan := *item.Plan
		plan.Mode, plan.Loan = models.GoalPlanInstallments, &loan
	}
	if plan.Loan == nil {
		// A fifth down, to start from the usual bank loan.
		plan.Loan = &models.InstallmentPlan{Strategy: models.StrategyFlat, DownPayment: 20, DownPaymentPct: true}
	}
	fd := mapGoalPlanToFD(plan, nil)
	fd.From = goalFrom(r)
	return views.GoalPlanForm(gl, fd, money, h.goalPlanOptions(gl, plan)), nil
}

// GoalPlanPreview compares saving the whole price with a down payment and a
// loan, for the plan being filled.
func (h Handler) GoalPlanPreview(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	gl, money, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	plan, err := mapFDToGoalPlan(r)
	if err != nil {
		return views.GoalPlanHint("Check the numbers, some aren't valid."), nil
	}
	return views.GoalPlanPreview(gl, money, h.goalPlanOptions(gl, plan)), nil
}

// goalLoanProblems are the loan's problems for the price. Its months can be
// left out, fitted to the monthly amount.
func goalLoanProblems(loan *models.InstallmentPlan, price float64) errs.ValidationErrors {
	v := *loan
	if v.Months == 0 && (v.Strategy != models.StrategyKnownAmount || v.Amount <= 0) {
		v.Months = 1
	}
	return v.Validate(price)
}

// goalPlanOptions works the goal out by saving the whole price and by the
// plan's loan, from its progress.
func (h Handler) goalPlanOptions(gl *models.Goal, plan *models.GoalPlan) views.GoalPlanOptions {
	now := time.Now()
	opts := views.GoalPlanOptions{Mode: plan.Mode, Monthly: plan.Monthly}
	if plan.Monthly <= 0 {
		return opts
	}
	// As if saved now, as saving it starts it over.
	plan = &models.GoalPlan{Mode: plan.Mode, Monthly: plan.Monthly, Loan: plan.Loan, StartsOn: now, StartSaved: gl.Progress()}

	save := *plan
	save.Mode = models.GoalPlanSave
	opts.Save, _ = save.Project(gl.Price, gl.Progress(), now)

	if plan.Loan == nil {
		return opts
	}
	if ve := goalLoanProblems(plan.Loan, gl.Price); len(ve) > 0 {
		opts.LoanProblem = planProblem(ve)
		return opts
	}
	loan := *plan
	loan.Mode = models.GoalPlanInstallments
	pr, err := loan.Project(gl.Price, gl.Progress(), now)
	switch {
	case errors.Is(err, models.ErrPlanMonthlyTooLow):
		opts.LoanProblem = "Your monthly amount doesn't cover this loan's payments, even over 50 years."
	case err != nil:
		opts.LoanProblem = err.Error()
	default:
		opts.Loan = pr
	}
	return opts
}

// SetGoalPlan saves the goal's plan, starting it over from its progress.
func (h Handler) SetGoalPlan(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	gl, money, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	plan, err := mapFDToGoalPlan(r)
	fd := mapGoalPlanToFD(plan, nil)
	fd.Monthly.Value = r.FormValue("monthly")
	failed := false
	if err != nil {
		fd.Monthly.Error, failed = "Check the numbers, some aren't valid", true
	} else if plan.Monthly <= 0 {
		fd.Monthly.Error, failed = "Value has to be more than 0", true
	}
	if !failed && plan.Mode == models.GoalPlanInstallments {
		if ve := goalLoanProblems(plan.Loan, gl.Price); len(ve) > 0 {
			fd = mapGoalPlanToFD(plan, ve)
			fd.Monthly.Value = r.FormValue("monthly")
			failed = true
		} else if _, err := plan.LoanSchedule(gl.Price); err != nil {
			fd.Monthly.Error, failed = "It doesn't cover the loan's payments", true
		}
	}
	if failed {
		fd.From = goalFrom(r)
		return views.GoalPlanFormFields(gl, fd, money, h.goalPlanOptions(gl, plan)), nil
	}

	if plan.Mode == models.GoalPlanSave {
		// Only the loan's mode keeps it.
		plan.Loan = nil
	}
	plan.StartsOn = time.Now()
	plan.StartSaved = gl.Progress()
	if err := h.repo.SetGoalPlan(r.Context(), gl.ID, plan); err != nil {
		l.Error("saving goal plan", zap.Error(err), zap.String("id", gl.ID))
		return nil, err
	}
	l.Info("saved goal plan", zap.String("id", gl.ID), zap.Any("plan", plan))

	goalSaved(w, r, gl.ID)
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	return h.goalAnswer(w, r)
}

// DeleteGoalPlan drops the goal's plan.
func (h Handler) DeleteGoalPlan(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.SetGoalPlan(r.Context(), id, nil); err != nil {
		return nil, err
	}
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	return h.Goal(w, r)
}

// goalPlanCards are the plans' projections and next deposits of the goals
// in progress, by goal id.
func (h Handler) goalPlanCards(r *http.Request, gls []models.Goal) (map[string]*views.GoalPlanCard, error) {
	cards := map[string]*views.GoalPlanCard{}
	planned := false
	for i := range gls {
		planned = planned || (gls[i].Plan != nil && gls[i].FulfilledAt.IsZero())
	}
	if !planned {
		return cards, nil
	}

	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		return nil, err
	}
	now := time.Now()

	for i := range gls {
		gl := &gls[i]
		if gl.Plan == nil || !gl.FulfilledAt.IsZero() {
			continue
		}
		// Scheduled deposits are counted as made, so they're not asked again.
		card := &views.GoalPlanCard{}
		pr, err := gl.Plan.Project(gl.Price, gl.Progress()+gl.ScheduledDeposits(), now)
		if err != nil {
			card.Problem = "The plan doesn't work out anymore, edit it."
			cards[gl.ID] = card
			continue
		}
		card.Projection = pr
		card.Next, card.NextOn = pr.NextDeposit()
		if card.Next > 0 {
			card.Source = models.PickGoalSource(accs, card.Next, &gl.Currency)
			card.NextURL = goalNextDepositURL(gl, card)
		}
		cards[gl.ID] = card
	}
	return cards, nil
}

// goalNextDepositURL opens the deposit drawer filled with the plan's next
// deposit, from the picked account, dated today (even when it's next
// month's).
func goalNextDepositURL(gl *models.Goal, card *views.GoalPlanCard) string {
	q := url.Values{"dir": {string(views.GoalDeposit)}, "goal_amount": {formatFloatIfNonZero(card.Next)}}
	if card.Source.Account != nil {
		q.Set("account_id", card.Source.Account.ID)
	}
	return "/goals/" + gl.ID + "/money-form?" + q.Encode()
}

func mapGoalPlanToFD(plan *models.GoalPlan, ve errs.ValidationErrors) *views.GoalPlanFD {
	loan := plan.Loan
	if loan == nil {
		loan = &models.InstallmentPlan{}
	}
	return &views.GoalPlanFD{
		Monthly: form.FDInput{Value: formatFloatIfNonZero(plan.Monthly)},
		Mode:    form.FDInput{Value: plan.Mode.String()},
		Loan:    mapPlanToFD(loan, ve),
	}
}

// mapFDToGoalPlan reads the plan form: the monthly amount, the mode and the
// loan (always read, so switching modes keeps it).
func mapFDToGoalPlan(r *http.Request) (*models.GoalPlan, error) {
	monthly, err := parseFloatIfExists(r.FormValue("monthly"))
	if err != nil {
		return &models.GoalPlan{}, err
	}
	loan, err := mapFDToInstallmentPlan(r)
	return &models.GoalPlan{
		Mode:    models.GoalPlanModeFromString(r.FormValue("mode")),
		Monthly: monthly,
		Loan:    loan,
	}, err
}
