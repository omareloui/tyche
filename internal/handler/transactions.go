package handler

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-planner/internal/errs"
	"github.com/omareloui/financial-planner/internal/idgen"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"github.com/omareloui/financial-planner/internal/slicesutils"
	"github.com/omareloui/financial-planner/web/views"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		return nil, err
	}
	fd := h.mapTransactionToFD(defaultTransaction(currencies, kinds), new(errs.ValidationError))

	l := logger.FromCtx(r.Context())

	transactions, err := h.repo.GetTransactions(r.Context())
	if err != nil {
		l.Error("fetch transaction from db", zap.Error(err))
		return nil, err
	}

	l.Debug("transaction from data", zap.Any("fd", fd))
	l.Debug("transactions count", zap.Int("transactions_count", len(transactions)))

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	defaultCurrency := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

	comp := views.Transactions(balances, fd, transactions,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds),
		defaultCurrency.Enum.Symbol())

	return comp, nil
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	// TODO: use a transaction here
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()
	tx.CreatedAt = time.Now()

	sod := time.Now().Truncate(24 * time.Hour)
	if tx.ScheduledTo.Equal(sod) || tx.ScheduledTo.Before(sod) {
		tx.FulfilledOn = tx.CreatedAt
	}

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("create transaction", zap.Any("transaction", tx))

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}
	dfltCurr := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("create transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd := h.mapTransactionToFD(tx, valerr)

		return views.CreateTransactionForm(fd,
			h.repo.CurrenciesToMap(currencies),
			h.repo.TransactionKindsToMap(kinds),
			dfltCurr.Enum.Symbol()), nil
	}

	if err := h.repo.CreateTransaction(r.Context(), tx); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")

	c := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.ID == tx.Currency.ID })
	tx.Currency = *c
	k := slicesutils.Find(kinds, func(k models.TransactionKind) bool { return k.ID == tx.Kind.ID })
	tx.Kind = *k

	if err = renderToBody(w, r, views.TransactionOOB(tx)); err != nil {
		l.Error("rendering transaction OOB")
		return nil, err
	}
	l.Debug("rendered transaction OOB")

	if !tx.FulfilledOn.IsZero() {
		if err := h.applyTransactionToBalance(r, tx); err != nil {
			return nil, err
		}
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	fd := h.mapTransactionToFD(defaultTransaction(currencies, kinds), new(errs.ValidationError))

	return views.CreateTransactionForm(fd,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds),
		dfltCurr.Enum.Symbol()), nil
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

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("get edit transaction", zap.Any("transaction", tx))

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))

	return views.EditTransaction(tx, fd,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds)), nil
}

func (h Handler) EditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	// TODO: use a transaction here
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}
	tx.ID = id

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("update transaction", zap.Any("new transaction", tx))

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("update transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd := h.mapTransactionToFD(tx, valerr)
		fd.Amount.Value = r.FormValue("amount")

		currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
		if err != nil {
			l.Error("fetching currencies and kinds")
			return nil, err
		}

		return views.EditTransaction(tx, fd,
			h.repo.CurrenciesToMap(currencies),
			h.repo.TransactionKindsToMap(kinds)), nil
	}

	oldTx, err := h.repo.GetTransaction(r.Context(), tx.ID)
	if err != nil {
		return nil, err
	}

	tx.FulfilledOn = oldTx.FulfilledOn
	tx.CreatedAt = oldTx.CreatedAt

	if err := h.repo.UpdateTransaction(r.Context(), tx); err != nil {
		l.Error("updating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("updated transaction")

	txKind, err := h.repo.GetTransactionKind(r.Context(), tx.Kind.ID)
	if err != nil {
		l.Error("fetching txKind", zap.Error(err))
		return nil, err
	}
	txCurrency, err := h.repo.GetCurrency(r.Context(), tx.Currency.ID)
	if err != nil {
		l.Error("fetching txCurrency", zap.Error(err))
		return nil, err
	}

	tx.Kind = *txKind
	tx.Currency = *txCurrency

	if !oldTx.FulfilledOn.IsZero() {
		if err := h.undoTransactionOnBalance(r, oldTx); err != nil {
			return nil, err
		}
		if err := h.applyTransactionToBalance(r, tx); err != nil {
			return nil, err
		}
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

	if err := h.repo.FulfillTransaction(r.Context(), id, time.Now()); err != nil {
		l.Error("fulfilling transaction", zap.Error(err))
		return nil, err
	}
	l.Info("fulfilled transaction")

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction", zap.Any("tx", tx))

	if err := h.applyTransactionToBalance(r, tx); err != nil {
		return nil, err
	}
	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
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

	if err := h.repo.DeleteTransaction(r.Context(), id); err != nil {
		l.Error("deleting transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted transaction")

	if !tx.FulfilledOn.IsZero() {
		if err := h.undoTransactionOnBalance(r, tx); err != nil {
			return nil, err
		}
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (h Handler) applyTransactionToBalance(r *http.Request, tx *models.Transaction) error {
	l := logger.FromCtx(r.Context())
	upBalance := h.repo.RemoveFromBalance
	if tx.Kind.IncreaseBalance {
		upBalance = h.repo.AddToBalance
	}
	if err := upBalance(r.Context(), tx.Currency.ID, tx.Amount); err != nil {
		l.Error("applying transaction to balance", zap.Error(err))
		return err
	}
	l.Info("applied transaction to balance")
	return nil
}

func (h Handler) undoTransactionOnBalance(r *http.Request, tx *models.Transaction) error {
	l := logger.FromCtx(r.Context())
	upBalance := h.repo.AddToBalance
	if tx.Kind.IncreaseBalance {
		upBalance = h.repo.RemoveFromBalance
	}
	if err := upBalance(r.Context(), tx.Currency.ID, tx.Amount); err != nil {
		l.Error("undoing transaction on balance", zap.Error(err))
		return err
	}
	l.Info("undone transaction on balance")
	return nil
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

func (h Handler) getCurrenciesAndKinds(ctx context.Context) ([]models.Currency, []models.TransactionKind, error) {
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	knds, err := h.repo.GetTransactionKinds(ctx)
	if err != nil {
		return nil, nil, err
	}
	return curs, knds, nil
}

func (h Handler) mapTransactionToFD(t *models.Transaction, err *errs.ValidationError) *views.TransactionFD {
	return &views.TransactionFD{
		AmountFD: views.AmountFD{
			Amount:     views.FDInput{Value: formatIntIfNonZero(int(t.Amount)), Error: err.Errors.MsgFor("Amount")},
			CurrencyID: views.FDInput{Value: t.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		},
		KindID:      views.FDInput{Value: t.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		IsRecurrent: views.FDInput{Value: formatBooleanIfNonZero(t.IsRecurrent), Error: err.Errors.MsgFor("IsRecurrent")},
		ScheduledTo: views.FDInput{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
	}
}

func (h Handler) mapFDToTransaction(r *http.Request) (*models.Transaction, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	ScheduledTo, err := parseDateOnlyIfExists(r.FormValue("scheduled_to"))
	if err != nil {
		return nil, err
	}

	return &models.Transaction{
		Amount:      amount,
		Currency:    models.Currency{ID: r.FormValue("currency_id")},
		Kind:        models.TransactionKind{ID: r.FormValue("kind_id")},
		IsRecurrent: parseBoolean(r.FormValue("is_recurrent")),
		ScheduledTo: ScheduledTo,
	}, nil
}

func defaultTransaction(currencies []models.Currency, kinds []models.TransactionKind) *models.Transaction {
	dfltCurrIdx := slices.IndexFunc(currencies, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })
	dfltKindIdx := slices.IndexFunc(kinds, func(knd models.TransactionKind) bool { return knd.Kind == models.DfltKind })

	return &models.Transaction{
		Amount:      0,
		Currency:    models.Currency{ID: currencies[dfltCurrIdx].ID},
		Kind:        models.TransactionKind{ID: kinds[dfltKindIdx].ID},
		IsRecurrent: false,
		ScheduledTo: time.Now(),
	}
}
