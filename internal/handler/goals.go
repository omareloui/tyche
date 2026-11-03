package handler

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) Goals(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goals", zap.Int("count", len(gls)))

	claims := getClaims(r.Context())
	return views.Goals(claims, b, h.mapGlToFD(dfltGoal(&claims.DefaultCurrency), &errs.ValidationError{}), gls, curs), nil
}

func (h Handler) Goal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		l.Error("fetching goal", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goal")

	return views.Goal(gl), nil
}

func (h Handler) CreateGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	gl, err := h.mapFDToGoal(r)
	if err != nil {
		l.Error("parsing form to goal", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form to goal", zap.Any("goal", gl))

	gl.ID = idgen.New()
	gl.CreatedAt = time.Now()

	curs, err := h.repo.GetAllCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	shownCurs := slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown })

	claims := getClaims(r.Context())
	renderForm := func(valerr *errs.ValidationError) templ.Component {
		return views.CreateGoalForm(h.mapGlToFD(gl, valerr), shownCurs, &claims.DefaultCurrency)
	}

	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("create goal validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return renderForm(&valerr), nil
		}
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if err = h.repo.CreateGoal(r.Context(), gl); err != nil {
		var valerr *errs.ValidationError
		if errors.As(err, &valerr) {
			return renderForm(valerr), nil
		}
		l.Error("creating goal", zap.Error(err))
		return nil, err
	}
	l.Info("created goal", zap.Any("goal", gl))
	drawerCreated(w)

	if gl, err = h.repo.GetGoal(r.Context(), gl.ID); err != nil {
		return nil, err
	}
	if err = renderToBody(w, r, views.GoalOOB(gl)); err != nil {
		l.Error("rendering OOB to body", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered goal")

	if err = h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	return views.CreateGoalForm(h.mapGlToFD(dfltGoal(&claims.DefaultCurrency), &errs.ValidationError{}), shownCurs, &claims.DefaultCurrency), nil
}

func (h Handler) GetEditGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		l.Error("fetching goal", zap.Error(err))
		return nil, err
	}
	l.Info("fetched goal")

	claims := getClaims(r.Context())
	return views.EditGoal(id, h.mapGlToFD(gl, &errs.ValidationError{}), curs, &claims.DefaultCurrency), nil
}

func (h Handler) EditGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	gl, err := h.mapFDToGoal(r)
	if err != nil {
		l.Error("parsing form to goal", zap.Error(err))
		return nil, err
	}
	l.Debug("parsed form to goal", zap.Any("goal", gl))

	gl.ID = id

	curs, err := h.repo.GetAllCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies for updating goal", zap.Error(err))
		return nil, err
	}
	shownCurs := slicesutils.Filter(curs, func(cur models.Currency) bool { return cur.IsShown })

	if cur := slicesutils.Find(curs, func(cur models.Currency) bool { return cur.ID == gl.Currency.ID }); cur != nil {
		gl.Currency = *cur
	}

	claims := getClaims(r.Context())
	renderForm := func(valerr *errs.ValidationError) templ.Component {
		return views.EditGoalForm(id, h.mapGlToFD(gl, valerr), shownCurs, &claims.DefaultCurrency)
	}

	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("update goal validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return renderForm(&valerr), nil
		}
		l.Error("sanitizing goal", zap.Error(err))
		return nil, err
	}

	if err = h.repo.UpdateGoal(r.Context(), gl); err != nil {
		var valerr *errs.ValidationError
		if errors.As(err, &valerr) {
			return renderForm(valerr), nil
		}
		l.Error("updating goal", zap.Error(err))
		return nil, err
	}
	l.Info("updated goal", zap.Any("goal", gl))
	drawerSaved(w, "#"+views.GoalID(id), "outerHTML")

	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	return h.Goal(w, r)
}

// goalMoneyDir reads which way money moves in a goal's money form.
func goalMoneyDir(r *http.Request) views.GoalMoneyDir {
	switch d := views.GoalMoneyDir(r.FormValue("dir")); d {
	case views.GoalWithdraw, views.GoalDeleteMove:
		return d
	}
	return views.GoalDeposit
}

// goalMoneyAccounts are the accounts money can move between with a goal.
func goalMoneyAccounts(accs []models.Account) []models.Account {
	return slicesutils.Filter(accs, func(a models.Account) bool {
		return !a.IsArchive && a.Kind.Enum != models.AccKindDebt && a.Kind.Enum != models.AccKindGamyaa
	})
}

// goalAndAccounts is the goal of the request's id with the accounts its money
// can move from or to.
func (h Handler) goalAndAccounts(r *http.Request) (*models.Goal, []models.Account, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, nil, errs.ErrInvalidID
	}
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		return nil, nil, err
	}
	return gl, goalMoneyAccounts(accs), nil
}

// GetGoalMoneyForm is the drawer putting money in the goal (deposit) or
// taking some out (withdraw).
func (h Handler) GetGoalMoneyForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	gl, accs, err := h.goalAndAccounts(r)
	if err != nil {
		return nil, err
	}
	dir := goalMoneyDir(r)
	fd := &views.GoalMoneyFD{ScheduledTo: form.FDInput{Value: time.Now().Format(time.DateOnly)}}
	if a := defaultGoalAccount(accs); a != nil {
		fd.AccountID.Value = a.ID
	}
	if dir == views.GoalWithdraw {
		fd.Amount.Value = formatFloatIfNonZero(gl.Balance())
		fd.OtherAmount.Value = goalOtherAmount(gl, accs, fd.AccountID.Value, dir, gl.Balance())
	}
	return views.GoalMoneyForm(gl, dir, fd, accs), nil
}

func defaultGoalAccount(accs []models.Account) *models.Account {
	if a := slicesutils.Find(accs, func(a models.Account) bool { return a.IsDefault }); a != nil {
		return a
	}
	if len(accs) > 0 {
		return &accs[0]
	}
	return nil
}

// goalOtherAmount is amount converted to the other side's currency
// (formatted for the form), "" when there's no rate or no conversion.
func goalOtherAmount(gl *models.Goal, accs []models.Account, accID string, dir views.GoalMoneyDir, amount float64) string {
	a := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == accID })
	if a == nil || a.Currency.String == gl.Currency.String || amount == 0 {
		return ""
	}
	from, to := a.Currency, gl.Currency
	if dir != views.GoalDeposit {
		from, to = gl.Currency, a.Currency
	}
	rate := from.FxRate(to.String)
	if rate == 0 {
		return ""
	}
	return formatFloatIfNonZero(numsutils.Round(amount * rate))
}

// GoalMoneyInputs renders the money form's amounts again for the picked
// account (so currency).
func (h Handler) GoalMoneyInputs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	gl, accs, err := h.goalAndAccounts(r)
	if err != nil {
		return nil, err
	}
	dir := goalMoneyDir(r)
	fd := &views.GoalMoneyFD{
		Amount:      form.FDInput{Value: r.FormValue("amount")},
		AccountID:   form.FDInput{Value: r.FormValue("account_id")},
		OtherAmount: form.FDInput{Value: r.FormValue("to_amount")},
	}
	if r.Header.Get("HX-Trigger-Name") == "account_id" || fd.OtherAmount.Value == "" {
		amt, _ := parseFloatIfExists(fd.Amount.Value)
		fd.OtherAmount.Value = goalOtherAmount(gl, accs, fd.AccountID.Value, dir, amt)
	}
	return views.GoalMoneyInputs(gl, dir, fd, accs), nil
}

// goalMoneyTx builds the transfer of a goal's money form: from the account
// to the goal's (deposit) or back (withdraw, moving it out before deleting
// the goal). It answers the form's errors, if any.
func (h Handler) goalMoneyTx(r *http.Request, gl *models.Goal, accs []models.Account, dir views.GoalMoneyDir) (*models.Transaction, *views.GoalMoneyFD) {
	fd := &views.GoalMoneyFD{
		Amount:      form.FDInput{Value: r.FormValue("amount")},
		AccountID:   form.FDInput{Value: r.FormValue("account_id")},
		OtherAmount: form.FDInput{Value: r.FormValue("to_amount")},
		ScheduledTo: form.FDInput{Value: r.FormValue("scheduled_to")},
	}
	failed := false
	fail := func(in *form.FDInput, msg string) {
		in.Error = msg
		failed = true
	}

	amount, err := parseFloatIfExists(fd.Amount.Value)
	if dir == views.GoalDeleteMove {
		amount = gl.Balance()
	}
	if err != nil || amount <= 0 {
		fail(&fd.Amount, "Value has to be more than 0")
	}

	acc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == fd.AccountID.Value })
	if acc == nil {
		fail(&fd.AccountID, "This field is required")
	}

	otherAmount := amount
	if acc != nil && acc.Currency.String != gl.Currency.String {
		otherAmount, err = parseFloatIfExists(fd.OtherAmount.Value)
		if err != nil || otherAmount <= 0 {
			fail(&fd.OtherAmount, "Value has to be more than 0")
		}
	}

	day := time.Now()
	if dir == views.GoalDeposit {
		if day, err = parseDateOnlyIfExists(fd.ScheduledTo.Value); err != nil || day.IsZero() {
			fail(&fd.ScheduledTo, "This field is required")
		}
	} else if amount > gl.Balance() {
		fail(&fd.Amount, errs.ValidationField{Tag: "gt_goal_balance", Param: gl.ViewBalance()}.String())
	}

	if failed {
		return nil, fd
	}

	tx := &models.Transaction{
		ID:          idgen.New(),
		Kind:        models.TransferTxKind,
		Amount:      amount,
		ToAmount:    otherAmount,
		Description: gl.Name,
		ScheduledTo: day,
		CreatedAt:   time.Now(),
	}
	if dir == views.GoalDeposit {
		tx.Account, tx.ToAccount = *acc, gl.Account
	} else {
		tx.Account, tx.ToAccount = *gl.Account, acc
	}
	if !tx.ScheduledTo.After(tx.CreatedAt) {
		tx.FulfilledAt = tx.CreatedAt
	}
	return tx, nil
}

// MoveGoalMoney puts money in the goal or takes some out, a transfer
// between an account and the goal's.
func (h Handler) MoveGoalMoney(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	gl, accs, err := h.goalAndAccounts(r)
	if err != nil {
		return nil, err
	}
	dir := goalMoneyDir(r)
	if dir == views.GoalDeleteMove {
		return nil, errs.ErrInvalidID
	}

	tx, fd := h.goalMoneyTx(r, gl, accs, dir)
	if fd != nil {
		return views.GoalMoneyFormFields(gl, dir, fd, accs), nil
	}

	if err = h.repo.CreateTransaction(r.Context(), tx, nil, nil); err != nil {
		l.Error("moving goal money", zap.Any("tx", tx), zap.String("goal", gl.ID), zap.Error(err))
		return nil, err
	}
	drawerSaved(w, "#"+views.GoalID(gl.ID), "outerHTML")

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	return h.Goal(w, r)
}

// goalBuyDeps are the expense categories and the counterparties for the
// goal's purchase.
func (h Handler) goalBuyDeps(r *http.Request) ([]models.Category, []models.Counterparty, error) {
	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, nil, err
	}
	cats = slicesutils.Filter(cats, func(c models.Category) bool { return slices.Contains(c.Kinds, models.ExpenseTxKind) })
	return cats, cps, nil
}

// GetGoalFulfillForm is the drawer recording the goal's purchase.
func (h Handler) GetGoalFulfillForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}
	cats, cps, err := h.goalBuyDeps(r)
	if err != nil {
		return nil, err
	}
	fd := &views.GoalBuyFD{
		Amount:      form.FDInput{Value: formatFloatIfNonZero(numsutils.Round(min(gl.Balance(), gl.Price)))},
		Description: form.FDInput{Value: gl.Name},
		ScheduledTo: form.FDInput{Value: time.Now().Format(time.DateOnly)},
	}
	return views.GoalBuyForm(gl, fd, cats, cps), nil
}

// FulfillGoal records buying the goal, an expense out of its account, and
// marks it achieved.
func (h Handler) FulfillGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	id := r.PathValue("id")
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !gl.FulfilledAt.IsZero() {
		return h.Goal(w, r)
	}
	cats, cps, err := h.goalBuyDeps(r)
	if err != nil {
		return nil, err
	}

	fd := &views.GoalBuyFD{
		Amount:         form.FDInput{Value: r.FormValue("amount")},
		CategoryID:     form.FDInput{Value: r.FormValue("category_id")},
		CounterpartyID: form.FDInput{Value: r.FormValue("counterparty_id")},
		Description:    form.FDInput{Value: r.FormValue("description")},
		ScheduledTo:    form.FDInput{Value: r.FormValue("scheduled_to")},
	}
	failed := false
	amount, err := parseFloatIfExists(fd.Amount.Value)
	switch {
	case err != nil || amount <= 0:
		fd.Amount.Error, failed = "Value has to be more than 0", true
	case amount > numsutils.Round(gl.Balance()):
		fd.Amount.Error, failed = errs.ValidationField{Tag: "gt_goal_balance", Param: gl.ViewBalance()}.String(), true
	}
	day, err := parseDateOnlyIfExists(fd.ScheduledTo.Value)
	switch {
	case err != nil || day.IsZero():
		fd.ScheduledTo.Error, failed = "This field is required", true
	case day.After(time.Now()):
		fd.ScheduledTo.Error, failed = errs.ValidationField{Tag: "future_date"}.String(), true
	}
	if failed {
		return views.GoalBuyFormFields(gl, fd, cats, cps), nil
	}

	purchase := &models.Transaction{
		ID:          idgen.New(),
		Kind:        models.ExpenseTxKind,
		Account:     *gl.Account,
		Amount:      amount,
		Description: fd.Description.Value,
		ScheduledTo: day,
		FulfilledAt: time.Now(),
		CreatedAt:   time.Now(),
	}
	if c := slicesutils.Find(cats, func(c models.Category) bool { return c.ID == fd.CategoryID.Value }); c != nil {
		purchase.Category = c
	}
	if cp := slicesutils.Find(cps, func(c models.Counterparty) bool { return c.ID == fd.CounterpartyID.Value }); cp != nil {
		purchase.Counterparty = cp
		applyCounterpartyDefaultCategory(purchase, cps)
	}

	if err := h.repo.FulfillGoal(r.Context(), id, purchase); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return nil, err
	}
	// The card moves to the achieved section.
	drawerSaved(w, "#"+views.GoalID(id), "outerHTML")
	return h.moveGoal(w, r, id)
}

// UnfulfillGoal undoes buying the goal: its purchase is deleted and the
// money goes back in it.
func (h Handler) UnfulfillGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.UnfulfillGoal(r.Context(), id); err != nil {
		return nil, err
	}
	return h.moveGoal(w, r, id)
}

// moveGoal moves the goal's card to its section after (un)fulfilling it.
// The card itself gets removed by the empty response.
func (h Handler) moveGoal(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	gl, err := h.repo.GetGoal(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, views.GoalOOB(gl)); err != nil {
		return nil, err
	}
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) renderGoalsSummaryOOB(w http.ResponseWriter, r *http.Request) error {
	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching goals for summary", zap.Error(err))
		return err
	}
	claims := getClaims(r.Context())
	return renderToBody(w, r, views.GoalsSummaryOOB(gls, &claims.DefaultCurrency))
}

// GetGoalDeleteForm is the drawer deleting a goal with money left in it,
// moving the money to an account first.
func (h Handler) GetGoalDeleteForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	gl, accs, err := h.goalAndAccounts(r)
	if err != nil {
		return nil, err
	}
	fd := &views.GoalMoneyFD{}
	if a := defaultGoalAccount(accs); a != nil {
		fd.AccountID.Value = a.ID
	}
	fd.OtherAmount.Value = goalOtherAmount(gl, accs, fd.AccountID.Value, views.GoalDeleteMove, gl.Balance())
	return views.GoalMoneyForm(gl, views.GoalDeleteMove, fd, accs), nil
}

// DeleteGoal deletes the goal, moving what's left in it to the picked
// account. Its account is kept (deleted) for its transactions' history.
func (h Handler) DeleteGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	gl, accs, err := h.goalAndAccounts(r)
	if err != nil {
		return nil, err
	}

	var move *models.Transaction
	if numsutils.Round(gl.Balance()) > 0 {
		var fd *views.GoalMoneyFD
		if move, fd = h.goalMoneyTx(r, gl, accs, views.GoalDeleteMove); fd != nil {
			return views.GoalMoneyFormFields(gl, views.GoalDeleteMove, fd, accs), nil
		}
	}

	if err := h.repo.DeleteGoal(r.Context(), gl, move); err != nil {
		l.Error("deleting goal", zap.Error(err), zap.String("id", gl.ID))
		return nil, err
	}
	l.Info("deleted goal", zap.String("id", gl.ID))

	if move != nil {
		drawerSaved(w, "#"+views.GoalID(gl.ID), "outerHTML")
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h Handler) mapGlToFD(gl *models.Goal, err *errs.ValidationError) *views.GoalFD {
	return &views.GoalFD{
		Name: form.FDInput{Value: gl.Name, Error: err.Errors.MsgFor("Name")},
		AmountFD: &form.AmountFD{
			Amount:     form.FDInput{Value: formatFloatIfNonZero(gl.Price), Error: err.Errors.MsgFor("Price")},
			CurrencyID: form.FDInput{Value: gl.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		},
	}
}

func (h Handler) mapFDToGoal(r *http.Request) (*models.Goal, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	amt, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	return &models.Goal{
		Name:     r.FormValue("name"),
		Price:    amt,
		Currency: models.Currency{ID: r.FormValue("currency_id")},
	}, nil
}

func dfltGoal(dfltCur *models.Currency) *models.Goal {
	return &models.Goal{Currency: *dfltCur}
}
