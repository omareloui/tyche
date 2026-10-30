package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	fd := h.mapTransactionToFD(defaultTransaction(accs), new(errs.ValidationError))

	txs, err := h.repo.GetTransactions(r.Context())
	if err != nil {
		l.Error("fetch transaction from db", zap.Error(err))
		return nil, err
	}

	l.Debug("transaction from data", zap.Int("transactions_count", len(txs)))

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	return views.Transactions(getClaims(r.Context()), balances, txs, fd, accs, knds, cats, cps, gls), nil
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r, r.FormValue("is_recurrent") != "")
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()

	sod := time.Now().Truncate(24 * time.Hour)
	if tx.ScheduledTo.Equal(sod) || tx.ScheduledTo.Before(sod) {
		tx.FulfilledAt = time.Now()
	}

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	applyCounterpartyDefaultCategory(tx, cps)

	claims := getClaims(r.Context())

	fd, err := h.prepareTx(r, tx, accs, knds, cats, cps, gls)
	if fd != nil {
		return views.CreateTransactionForm(fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
	}
	if err != nil {
		return nil, err
	}

	var scheduledTxs []models.Transaction

	if tx.RecurrentTx != nil {
		tx.RecurrentTx.ID = idgen.New()
		tx.RecurrentTx.LastTransactionScheduledTo = time.Now()

		if err := tx.RecurrentTx.SanitizeAndValidate(h.sani, h.valid); err != nil {
			var valerr errs.ValidationError
			if errors.As(err, &valerr) {
				l.Error("validating recurrent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
				return views.CreateTransactionForm(h.mapTransactionToFD(tx, &valerr), accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
			}
			l.Error("sanitizing recurrent transaction", zap.Error(err))
			return nil, err
		}

		scheduledTxs = generateNextScheduledTransactions(tx.RecurrentTx, generateNextScheduledTransactionsCount)
	}

	if err := h.repo.CreateTransaction(r.Context(), tx, tx.RecurrentTx, scheduledTxs); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")

	// Re-render the whole list so new transactions land in their day group.
	txs, err := h.repo.GetTransactions(r.Context())
	if err != nil {
		l.Error("fetch transactions from db", zap.Error(err))
		return nil, err
	}
	if err = renderToBody(w, r, views.TransactionsListOOB(txs)); err != nil {
		l.Error("rendering transactions list OOB", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered transactions list OOB")

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	fd = h.mapTransactionToFD(defaultTransaction(accs), new(errs.ValidationError))
	return views.CreateTransactionForm(fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
}

func (h Handler) Transaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	l.Info("fetched transaction", zap.Any("tx", tx))

	return views.Transaction(tx), nil
}

func (h Handler) GetEditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}

	l.Debug("get edit transaction", zap.Any("transaction", tx))

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))
	fd.RecurrenceFD = nil

	claims := getClaims(r.Context())
	return views.EditTransaction(tx, fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
}

func (h Handler) EditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r, r.FormValue("is_recurrent") != "")
	if err != nil {
		return nil, err
	}
	tx.ID = id

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	fd, err := h.prepareTx(r, tx, accs, knds, cats, cps, gls)
	if fd != nil {
		claims := getClaims(r.Context())
		return views.EditTransaction(tx, fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency), nil
	}
	if err != nil {
		return nil, err
	}

	l.Debug("update transaction", zap.Any("new_transaction", tx))

	oldTx, err := h.repo.GetTransaction(r.Context(), tx.ID)
	if err != nil {
		return nil, err
	}

	tx.FulfilledAt = oldTx.FulfilledAt
	tx.CreatedAt = oldTx.CreatedAt

	if err := h.repo.UpdateTransaction(r.Context(), oldTx, tx); err != nil {
		l.Error("updating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("updated transaction")

	if !oldTx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return views.Transaction(tx), nil
}

func (h Handler) FulfillTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction", zap.Any("tx", tx))

	tx.FulfilledAt = time.Now()

	if err := h.repo.FulfillTransaction(r.Context(), tx); err != nil {
		return nil, err
	}
	l.Info("fulfilled transaction")

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	return views.Transaction(tx), nil
}

func (h Handler) MarkTransactionReviewed(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.MarkTransactionReviewed(r.Context(), id); err != nil {
		return nil, err
	}
	l.Info("marked transaction as reviewed")

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}

	return views.Transaction(tx), nil
}

func (h Handler) DeleteTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction", zap.Any("transaction", tx))

	if err := h.repo.DeleteTransaction(r.Context(), tx); err != nil {
		l.Error("deleting transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted transaction")

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (h Handler) getAndRenderBalancesOOB(w http.ResponseWriter, r *http.Request) error {
	l := logger.FromCtx(r.Context())
	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return err
	}
	l.Debug("fetched balances", zap.Any("balances", balances))

	if err = renderToBody(w, r, views.BalanceOOB(balances)); err != nil {
		l.Error("rendering balance OOB")
		return err
	}
	l.Debug("rendered balance OOB")
	return nil
}

func (h Handler) getTxDeps(ctx context.Context) ([]models.Account, []models.TxKind, []models.Category, []models.Counterparty, []models.Goal, error) {
	l := logger.FromCtx(ctx)
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		l.Error("error fetching accounts for dx deps", zap.Error(err))
		return nil, nil, nil, nil, nil, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		l.Error("error fetching categories for dx deps", zap.Error(err))
		return nil, nil, nil, nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		l.Error("error fetching counterparties for dx deps", zap.Error(err))
		return nil, nil, nil, nil, nil, err
	}
	l.Debug("fetched transactions dependencies")
	gls, err := h.repo.GetUnfulfilledGoals(ctx)
	if err != nil {
		l.Error("error fetching unfulfilled goals for dx deps", zap.Error(err))
		return nil, nil, nil, nil, nil, err
	}
	l.Debug("fetched transactions dependencies")
	return accs, models.GetTxKinds(), cats, cps, gls, nil
}

func (h Handler) mapTransactionToFD(t *models.Transaction, err *errs.ValidationError) *views.TransactionFD {
	fd := &views.TransactionFD{
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: form.AccountIFD{
				Amount:    form.FDInput{Value: formatFloatIfNonZero(t.Amount), Error: err.Errors.MsgFor("Amount")},
				AccountID: form.FDInput{Value: t.Account.ID, Error: err.Errors.MsgFor("AccountID")},
			},
			Kind: form.FDInput{Value: t.Kind.String(), Error: err.Errors.MsgFor("Kind")},
		},
		Description:  form.FDInput{Value: t.Description, Error: err.Errors.MsgFor("Description")},
		ScheduledTo:  form.FDInput{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
		RecurrenceFD: &views.RecurrenceFD{},
	}

	hasToAccount := t.Kind == models.DebtTxKind || t.Kind == models.TransferTxKind
	hasGoal := t.Kind == models.ExpenseTxKind

	if t.Category != nil {
		fd.CategoryID = &form.FDInput{Value: t.Category.ID, Error: err.Errors.MsgFor("CategoryID")}
	} else if !hasToAccount {
		fd.CategoryID = &form.FDInput{}
	}

	if t.Counterparty != nil {
		fd.CounterpartyID = &form.FDInput{Value: t.Counterparty.ID, Error: err.Errors.MsgFor("CounterpartyID")}
	} else if !hasToAccount {
		fd.CounterpartyID = &form.FDInput{}
	}

	if t.ToAccount != nil {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Value: formatFloatIfNonZero(t.ToAmount), Error: err.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Value: t.ToAccount.ID, Error: err.Errors.MsgFor("ToAccountID")},
		}
	} else if hasToAccount {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Error: err.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Error: err.Errors.MsgFor("ToAccountID")},
		}
	}

	if t.Goal != nil {
		fd.GoalID = &form.FDInput{Value: t.Goal.ID, Error: err.Errors.MsgFor("GoalID")}
	} else if hasGoal {
		fd.GoalID = &form.FDInput{Error: err.Errors.MsgFor("GoalID")}
	}

	if t.RecurrentTx != nil {
		fd.RecurrenceFD = &views.RecurrenceFD{
			IsRecurrent: form.FDInput{Value: "on"},
			Unit:        form.FDInput{Value: formatIntIfNonZero(int(t.RecurrentTx.Unit)), Error: err.Errors.MsgFor("Unit")},
			Every:       form.FDInput{Value: formatIntIfNonZero(int(t.RecurrentTx.Every)), Error: err.Errors.MsgFor("Every")},
			StopsOn:     form.FDInput{Value: formatDateOnlyIfNonZero(t.RecurrentTx.StopsAt), Error: err.Errors.MsgFor("StopsOn")},
		}
	}

	return fd
}

func (h Handler) mapFDToTransaction(r *http.Request, isRecurrent bool) (*models.Transaction, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	toAmount, err := parseFloatIfExists(r.FormValue("to_amount"))
	if err != nil {
		return nil, err
	}

	scheduledTo, err := parseDateOnlyIfExists(r.FormValue("scheduled_to"))
	if err != nil {
		return nil, err
	}

	knd := models.GetTxKindFromString(r.FormValue("kind"))

	var cat *models.Category
	if catId := r.FormValue("category_id"); catId != "" {
		cat = &models.Category{ID: catId, Kinds: []models.TxKind{knd.CategoryKind()}}
	}

	var cp *models.Counterparty
	if cpId := r.FormValue("counterparty_id"); cpId != "" {
		cp = &models.Counterparty{ID: cpId}
	}

	toAccId := r.FormValue("to_account_id")
	var toAcc *models.Account
	if toAccId != "" {
		toAcc = &models.Account{ID: toAccId}
	}

	glId := r.FormValue("goal_id")
	var gl *models.Goal
	if glId != "" {
		gl = &models.Goal{ID: glId}
	}

	var rt *models.RecurrentTx

	tx := &models.Transaction{
		Account:      models.Account{ID: r.FormValue("account_id")},
		Amount:       amount,
		Kind:         knd,
		Category:     cat,
		Counterparty: cp,
		ToAccount:    toAcc,
		ToAmount:     toAmount,
		Goal:         gl,
		Description:  r.FormValue("description"),
		ScheduledTo:  scheduledTo,
	}

	if isRecurrent {
		tx.RecurrentTx, err = h.mapFDAndTxToRecurrentTx(r, tx)
		l := logger.FromCtx(r.Context())
		if err != nil {
			l.Error("parsing recurrent transaction", zap.Error(err), zap.Any("recurrent tx", rt))
			return nil, err
		}
		l.Debug("parsed recurrent transaction")
	}

	return tx, nil
}

func defaultTransaction(accs []models.Account) *models.Transaction {
	dfltAcc := models.Account{}
	if len(accs) > 0 {
		_dfltAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.IsDefault })
		if _dfltAcc != nil {
			dfltAcc = *_dfltAcc
		} else {
			dfltAcc = accs[0]
		}
	}

	knd := models.ExpenseTxKind

	tx := &models.Transaction{
		Amount:      0,
		Account:     dfltAcc,
		Kind:        knd,
		ScheduledTo: time.Now(),
		Category:    &models.Category{},
	}

	return tx
}

func (h Handler) mapFDAndTxToRecurrentTx(r *http.Request, tx *models.Transaction) (*models.RecurrentTx, error) {
	every, err := parseIntIfExists(r.FormValue("every"))
	if err != nil {
		return nil, err
	}

	unit, err := parseIntIfExists(r.FormValue("unit"))
	if err != nil {
		return nil, err
	}

	stopsOn, err := parseDateOnlyIfExists(r.FormValue("stops_on"))
	if err != nil {
		return nil, err
	}

	return &models.RecurrentTx{
		Account:      tx.Account,
		Amount:       tx.Amount,
		Kind:         tx.Kind,
		Category:     tx.Category,
		Counterparty: tx.Counterparty,
		Description:  tx.Description,
		ToAccount:    tx.ToAccount,
		ToAmount:     tx.ToAmount,
		Goal:         tx.Goal,
		Unit:         models.RecurrentUnit(unit),
		Every:        uint(every),
		StartsAt:     tx.ScheduledTo,
		StopsAt:      stopsOn,
	}, nil
}

// applyCounterpartyDefaultCategory sets the counterparty's default category on
// a transaction without a category.
func applyCounterpartyDefaultCategory(tx *models.Transaction, cps []models.Counterparty) {
	if tx.Category != nil || tx.Counterparty == nil {
		return
	}
	cp := slicesutils.Find(cps, func(c models.Counterparty) bool { return c.ID == tx.Counterparty.ID })
	tx.Category = cp.DefaultCategoryFor(tx.Kind)
	if tx.RecurrentTx != nil && tx.RecurrentTx.Category == nil {
		tx.RecurrentTx.Category = tx.Category
	}
}

func (h Handler) prepareTx(r *http.Request, tx *models.Transaction,
	accs []models.Account, knds []models.TxKind,
	cats []models.Category, cps []models.Counterparty,
	gls []models.Goal,
) (*views.TransactionFD, error) {
	l := logger.FromCtx(r.Context())

	if tx.Kind == models.NilTxKind {
		valerr := &errs.ValidationError{Errors: errs.ValidationErrors{}}
		valerr.Errors["Kind"] = errs.ValidationField{Tag: "required"}
		l.Error("invalid tx kind")
		return h.mapTransactionToFD(tx, valerr), nil
	}

	a := slicesutils.Find(accs, func(ac models.Account) bool { return ac.ID == tx.Account.ID })
	if a == nil {
		valerr := &errs.ValidationError{Errors: errs.ValidationErrors{}}
		valerr.Errors["AccountID"] = errs.ValidationField{Tag: "required"}
		l.Error("account doesn't exist")
		return h.mapTransactionToFD(tx, valerr), nil
	}
	tx.Account = *a

	if c := slicesutils.Find(cats, func(c models.Category) bool { return tx.Category != nil && c.ID == tx.Category.ID }); c != nil {
		tx.Category = c
		tx.Category.Kinds = []models.TxKind{tx.Kind.CategoryKind()}
	}

	if cp := slicesutils.Find(cps, func(c models.Counterparty) bool { return tx.Counterparty != nil && c.ID == tx.Counterparty.ID }); cp != nil {
		tx.Counterparty = cp
	}

	if tx.Goal != nil {
		gl := slicesutils.Find(gls, func(gl models.Goal) bool { return gl.ID == tx.Goal.ID })
		if gl == nil {
			valerr := &errs.ValidationError{Errors: errs.ValidationErrors{}}
			valerr.Errors["GoalID"] = errs.ValidationField{Tag: "required"}
			l.Error("goal kind doesn't exist")
			return h.mapTransactionToFD(tx, valerr), nil
		}
		tx.Goal = gl
	}

	if err := tx.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			return h.mapTransactionToFD(tx, &valerr), nil
		}
		l.Error("sanitizing transaction", zap.Error(err))
		return nil, err
	}
	return nil, nil
}

func (h Handler) prepareCreatingTx(r *http.Request, tx *models.Transaction,
	accs []models.Account, knds []models.TxKind,
	cats []models.Category, cps []models.Counterparty,
	gls []models.Goal,
) (*views.TransactionFD, error) {
	tx.ID = idgen.New()
	tx.CreatedAt = time.Now()

	now := time.Now()
	if tx.ScheduledTo.Equal(now) || tx.ScheduledTo.Before(now) {
		tx.FulfilledAt = tx.CreatedAt
	}

	return h.prepareTx(r, tx, accs, knds, cats, cps, gls)
}

func (h Handler) parseTransactionAmountsAndKindFD(r *http.Request, accs []models.Account, knds []models.TxKind) (*views.TransactionAmountsAndKindFD, error) {
	accId := r.FormValue("account_id")
	kindString := r.FormValue("kind")

	fd := &views.TransactionAmountsAndKindFD{
		AccountIFD: form.AccountIFD{
			Amount:    form.FDInput{Value: r.FormValue("amount")},
			AccountID: form.FDInput{Value: accId},
		},
		Kind: form.FDInput{Value: kindString},
	}

	trigger := r.Header.Get("Hx-Trigger")

	kind := models.GetTxKindFromString(kindString)

	gl := r.FormValue("goal_id")
	if gl != "" {
		fd.GoalID = &form.FDInput{Value: gl}
	}

	if kind == models.TransferTxKind || kind == models.DebtTxKind {
		toAccId := r.FormValue("to_account_id")
		toAmt := r.FormValue("to_amount")

		isDebtTx := kind == models.DebtTxKind

		acc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == accId })
		toAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == toAccId })

		// Set to account
		if toAccId == "" || strings.HasPrefix(trigger, "kind_id") ||
			(strings.HasPrefix(trigger, "account_id") && toAccId == accId) {
			if nAcc := slicesutils.Find(accs, func(a models.Account) bool {
				isDebtAcc := a.Kind.Enum == models.AccKindDebt
				if a.ID == accId {
					return false
				}
				if isDebtTx {
					return isDebtAcc
				}
				return !isDebtAcc
			}); nAcc != nil {
				toAccId = nAcc.ID
				toAcc = nAcc
			}
		}

		// Set from account
		if strings.HasPrefix(trigger, "to_account_id") || toAccId == accId {
			if nAcc := slicesutils.Find(accs, func(a models.Account) bool {
				isDebtAcc := a.Kind.Enum == models.AccKindDebt
				if a.ID == toAccId {
					return false
				}
				if isDebtTx {
					return isDebtAcc
				}
				return !isDebtAcc
			}); nAcc != nil {
				fd.AccountIFD.AccountID.Value = nAcc.ID
				acc = nAcc
			}
		}

		// Set to amount
		if strings.HasPrefix(trigger, "amount") ||
			strings.HasPrefix(trigger, "kind_id") ||
			strings.HasPrefix(trigger, "to_account_id") ||
			strings.HasPrefix(trigger, "account_id") {
			fxRate := 1.0
			if acc != nil && toAcc != nil {
				fxRate, _ = fxrate.GetFxRate(acc.Currency.String, toAcc.Currency.String)
			}
			amt, _ := parseFloatIfExists(fd.AccountIFD.Amount.Value)
			toAmt = strconv.FormatFloat(amt/fxRate, 'f', 2, 64)
		}

		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Value: toAmt},
			AccountID: form.FDInput{Value: toAccId},
		}
	} else {
		fd.CategoryID = &form.FDInput{Value: r.FormValue("category_id")}
		fd.CounterpartyID = &form.FDInput{Value: r.FormValue("counterparty_id")}
	}

	return fd, nil
}
