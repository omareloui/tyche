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

// goalPlanEnv is a goal and what its plan's month's money is worked out
// from.
type goalPlanEnv struct {
	goal  *models.Goal
	ws    *models.WorkSettings
	cats  []models.Category
	goals []models.Goal
	start models.MonthStart
}

// milestones are the days the other goals' plans free money from.
func (e *goalPlanEnv) milestones(fd *views.GoalPlanFD) {
	fd.Milestones = models.PlanMilestones(e.goal, e.ws, e.cats, e.goals, e.start, time.Now())
	fd.Currency = e.goal.Currency
}

// money is the goal's month's money free when saving starts on on (today
// when zero or past).
func (e *goalPlanEnv) money(on time.Time) models.GoalPlanMoney {
	return models.NewGoalPlanMoney(e.goal, e.ws, e.cats, e.goals, e.start, time.Now(), on)
}

// goalPlanDeps are a goal and what its plan's money is worked out from.
func (h Handler) goalPlanDeps(r *http.Request) (*goalPlanEnv, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}
	return h.goalPlanEnvOf(r, gl)
}

func (h Handler) goalPlanEnvOf(r *http.Request, gl *models.Goal) (*goalPlanEnv, error) {
	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}
	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, err
	}
	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		return nil, err
	}
	return &goalPlanEnv{goal: gl, ws: ws, cats: cats, goals: gls, start: getClaims(r.Context()).MonthStart}, nil
}

// GetGoalPlanForm is the drawer planning how to save for the goal.
func (h Handler) GetGoalPlanForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	env, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}
	gl := env.goal

	plan := &models.GoalPlan{Monthly: env.money(time.Time{}).Free()}
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
	env.milestones(fd)
	return views.GoalPlanForm(gl, fd, env.money(plan.StartsOn), h.goalPlanOptions(gl, plan)), nil
}

// GoalPlanPreview compares saving the whole price with a down payment and a
// loan, for the plan being filled.
func (h Handler) GoalPlanPreview(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	env, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	plan, err := mapFDToGoalPlan(r)
	if err != nil {
		opts := views.GoalPlanOptions{Mode: models.GoalPlanModeFromString(r.FormValue("mode"))}
		return views.GoalPlanPreviewAnswer(env.goal, env.money(time.Time{}), opts, "Check the numbers, some aren't valid."), nil
	}
	return views.GoalPlanPreviewAnswer(env.goal, env.money(plan.StartsOn), h.goalPlanOptions(env.goal, plan), ""), nil
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
func (h Handler) goalPlanOptions(gl *models.Goal, plan *models.GoalPlan) (opts views.GoalPlanOptions) {
	now := time.Now()
	opts = views.GoalPlanOptions{Mode: plan.Mode, Monthly: plan.Monthly, Months: plan.Months}
	if !plan.IsSized() && plan.Monthly <= 0 {
		return opts
	}
	// As if saved now, as saving it starts it over, from its start day.
	starts := now
	if plan.StartsOn.After(now) {
		starts = plan.StartsOn
	}
	plan = &models.GoalPlan{Mode: plan.Mode, Monthly: plan.Monthly, Months: plan.Months, Loan: plan.Loan, StartsOn: starts, StartSaved: gl.Progress()}
	defer func() {
		// A sized plan's monthly amount is the picked option's.
		if pr := opts.Picked(); plan.IsSized() && pr != nil {
			opts.Monthly = pr.Plan.Monthly
		}
	}()

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

	env, err := h.goalPlanDeps(r)
	if err != nil {
		return nil, err
	}
	gl := env.goal
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	plan, err := mapFDToGoalPlan(r)
	keepTyped := func(fd *views.GoalPlanFD) {
		fd.By.Value = r.FormValue("plan_by")
		fd.Monthly.Value = r.FormValue("monthly")
		fd.Months.Value = r.FormValue("months")
		fd.StartsOn.Value = r.FormValue("starts_on")
	}
	fd := mapGoalPlanToFD(plan, nil)
	keepTyped(fd)
	if msg := h.checkGoalPlan(r, plan, err, gl.Price, gl.Progress(), fd); msg != "" {
		fd.From = goalFrom(r)
		if msg == planLoanProblem {
			fd = mapGoalPlanToFD(plan, goalLoanProblems(plan.Loan, gl.Price))
			keepTyped(fd)
			fd.From = goalFrom(r)
		}
		env.milestones(fd)
		return views.GoalPlanFormFields(gl, fd, env.money(plan.StartsOn), h.goalPlanOptions(gl, plan)), nil
	}

	if plan.Mode == models.GoalPlanSave {
		// Only the loan's mode keeps it.
		plan.Loan = nil
	}
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

// planLoanProblem tells checkGoalPlan's caller the loan's fields have the
// problems.
const planLoanProblem = "loan"

// checkGoalPlan checks a plan read from its form (err its parsing error)
// for a price, saved now, setting the problems on fd; on success it starts
// the plan (StartsOn the day saving starts, today at the earliest) and keeps
// the monthly amount a sized plan comes to, paying its loan. It answers the
// problem, "" when none.
func (h Handler) checkGoalPlan(r *http.Request, plan *models.GoalPlan, err error, price, saved float64, fd *views.GoalPlanFD) string {
	today := h.userToday(r.Context())
	sized := r.FormValue("plan_by") == views.PlanByMonths
	switch {
	case err != nil && errors.Is(err, errInvalidStartsOn):
		fd.StartsOn.Error = "Check the day"
	case err != nil:
		fd.Monthly.Error = "Check the numbers, some aren't valid"
		fd.Months.Error = fd.Monthly.Error
	case !plan.StartsOn.IsZero() && plan.StartsOn.Before(today):
		fd.StartsOn.Error = "Pick today or a day ahead"
	case sized && plan.Months <= 0:
		fd.Months.Error = "Value has to be more than 0"
	case !sized && plan.Monthly <= 0:
		fd.Monthly.Error = "Value has to be more than 0"
	}
	if fd.StartsOn.Error != "" || fd.Monthly.Error != "" || fd.Months.Error != "" {
		return "invalid"
	}
	if plan.Mode == models.GoalPlanInstallments && len(goalLoanProblems(plan.Loan, price)) > 0 {
		return planLoanProblem
	}

	if plan.StartsOn.IsZero() || plan.StartsOn.Before(time.Now()) {
		plan.StartsOn = time.Now()
	}
	plan.StartSaved = saved
	plan.Monthly = plan.Sized(price).Monthly
	switch {
	case plan.Monthly <= 0:
		fd.Months.Error = "It's saved already, plan it by a monthly amount"
	case plan.Mode == models.GoalPlanInstallments:
		if _, err := plan.LoanSchedule(price); err != nil {
			fd.Monthly.Error = "It doesn't cover the loan's payments"
			fd.Months.Error = fd.Monthly.Error
		}
	}
	if fd.Monthly.Error != "" || fd.Months.Error != "" {
		return "invalid"
	}
	return ""
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
	fd := &views.GoalPlanFD{
		By:      form.FDInput{Value: views.PlanByMonthly},
		Monthly: form.FDInput{Value: formatFloatIfNonZero(plan.Monthly)},
		Mode:    form.FDInput{Value: plan.Mode.String()},
		Loan:    mapPlanToFD(loan, ve),
	}
	if plan.IsSized() {
		fd.By.Value = views.PlanByMonths
		fd.Months.Value = formatIntIfNonZero(plan.Months)
	}
	// A plan starting later shows its day, one started already today.
	if plan.StartsOn.After(time.Now()) {
		fd.StartsOn.Value = plan.StartsOn.Format(time.DateOnly)
	}
	return fd
}

// mapFDToGoalPlan reads the plan form: the monthly amount, the mode and the
// loan (always read, so switching modes keeps it).
func mapFDToGoalPlan(r *http.Request) (*models.GoalPlan, error) {
	plan := &models.GoalPlan{Mode: models.GoalPlanModeFromString(r.FormValue("mode"))}
	var err error
	if plan.StartsOn, err = parseDateOnlyIfExists(r.FormValue("starts_on")); err != nil {
		return plan, errInvalidStartsOn
	}
	if r.FormValue("plan_by") == views.PlanByMonths {
		// Its monthly amount is worked out from the months.
		if plan.Months, err = parseIntIfExists(r.FormValue("months")); err != nil {
			return plan, err
		}
	} else if plan.Monthly, err = parseFloatIfExists(r.FormValue("monthly")); err != nil {
		return plan, err
	}
	plan.Loan, err = mapFDToInstallmentPlan(r)
	return plan, err
}

var errInvalidStartsOn = errors.New("invalid start day")
