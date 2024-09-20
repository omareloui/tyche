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
	l := logger.FromCtx(r.Context())

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched currencies and kinds", zap.Any("currencies", currencies), zap.Any("kinds", kinds))

	fd := h.mapTransactionToFD(defaultTransaction(currencies, kinds), new(errs.ValidationError))

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

	currencies, kinds, err := h.getCurrenciesAndKinds(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}
	dfltCurr := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })

	c := slicesutils.Find(currencies, func(cur models.Currency) bool { return cur.ID == tx.Currency.ID })
	tx.Currency = *c
	k := slicesutils.Find(kinds, func(k models.TransactionKind) bool { return k.ID == tx.Kind.ID })
	tx.Kind = *k

	var ftx *models.FrequentTx
	var scheduledTxs []models.Transaction

	if r.FormValue("is_frequent") != "" {
		ftx, err = h.mapFDToTransactionFrequency(r, tx)
		if err != nil {
			l.Error("parsing frequent transaction", zap.Error(err), zap.Any("frequent tx", ftx))
			return nil, err
		}
		l.Debug("parsed frequent transaction", zap.Any("frequent tx", ftx))

		ftx.ID = idgen.New()
		ftx.CreatedAt = time.Now()
		ftx.LastTransactionCreatedAt = time.Now()

		if err := h.sani.SanitizeStruct(ftx); err != nil {
			return nil, errs.ErrSanitizer
		}

		if valerr := h.valid.Validate(ftx); valerr != nil {
			l.Error("frequent transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			fd := h.mapTransactionToFD(tx, valerr)

			fd.FrequencyFD = h.parseFrequencyFD(r, valerr)

			return views.CreateTransactionForm(fd,
				h.repo.CurrenciesToMap(currencies),
				h.repo.TransactionKindsToMap(kinds),
				dfltCurr.Enum.Symbol()), nil
		}

		scheduledTxs = generateNextScheduledTransactions(ftx, 5)
	}

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	l.Debug("transaction", zap.Any("transaction", tx))

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("create transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd := h.mapTransactionToFD(tx, valerr)

		return views.CreateTransactionForm(fd,
			h.repo.CurrenciesToMap(currencies),
			h.repo.TransactionKindsToMap(kinds),
			dfltCurr.Enum.Symbol()), nil
	}

	if err := h.repo.CreateTransaction(r.Context(), tx, ftx, scheduledTxs); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")

	if err = renderToBody(w, r, views.TransactionOOB(tx)); err != nil {
		l.Error("rendering transaction OOB")
		return nil, err
	}
	l.Debug("rendered transaction OOB")

	for _, scheduledTx := range scheduledTxs {
		if err = renderToBody(w, r, views.TransactionOOB(&scheduledTx)); err != nil {
			l.Error("rendering scheduled transaction OOB", zap.Error(err), zap.Any("tx", &scheduledTx))
			return nil, err
		}
		l.Debug("rendered scheduled transaction OOB")
	}

	if !tx.FulfilledOn.IsZero() {
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
	fd.FrequencyFD = nil

	return views.EditTransaction(tx, fd,
		h.repo.CurrenciesToMap(currencies),
		h.repo.TransactionKindsToMap(kinds)), nil
}

func (h Handler) EditTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	if err := h.repo.UpdateTransaction(r.Context(), oldTx, tx); err != nil {
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

	if err := h.repo.ApplyTransactionToBalance(r.Context(), tx); err != nil {
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
		if err := h.repo.UndoTransactionToBalance(r.Context(), tx); err != nil {
			return nil, err
		}
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

func (h Handler) getCurrenciesAndKinds(ctx context.Context) ([]models.Currency, []models.TransactionKind, error) {
	l := logger.FromCtx(ctx)

	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		l.Error("error fetching currencies", zap.Error(err))
		return nil, nil, err
	}
	knds, err := h.repo.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("error fetching transaction kinds", zap.Error(err))
		return nil, nil, err
	}
	l.Debug("fetched currencies and transaction kinds")
	return curs, knds, nil
}

func (h Handler) mapTransactionToFD(t *models.Transaction, err *errs.ValidationError) *views.TransactionFD {
	return &views.TransactionFD{
		AmountFD: views.AmountFD{
			Amount:     views.FDInput{Value: formatIntIfNonZero(int(t.Amount)), Error: err.Errors.MsgFor("Amount")},
			CurrencyID: views.FDInput{Value: t.Currency.ID, Error: err.Errors.MsgFor("Currency.ID")},
		},
		Description: views.FDInput{Value: t.Description, Error: err.Errors.MsgFor("Description")},
		KindID:      views.FDInput{Value: t.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		ScheduledTo: views.FDInput{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
		FrequencyFD: &views.FrequencyFD{},
	}
}

func (h Handler) mapFDToTransaction(r *http.Request) (*models.Transaction, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	scheduledTo, err := parseDateOnlyIfExists(r.FormValue("scheduled_to"))
	if err != nil {
		return nil, err
	}

	return &models.Transaction{
		Amount:      amount,
		Currency:    models.Currency{ID: r.FormValue("currency_id")},
		Kind:        models.TransactionKind{ID: r.FormValue("kind_id")},
		Description: r.FormValue("description"),
		ScheduledTo: scheduledTo,
	}, nil
}

func defaultTransaction(currencies []models.Currency, kinds []models.TransactionKind) *models.Transaction {
	dfltCurrIdx := slices.IndexFunc(currencies, func(cur models.Currency) bool { return cur.Enum == models.DfltCurrency })
	dfltKindIdx := slices.IndexFunc(kinds, func(knd models.TransactionKind) bool { return knd.Kind == models.DfltKind })

	return &models.Transaction{
		Amount:      0,
		Currency:    models.Currency{ID: currencies[dfltCurrIdx].ID},
		Kind:        models.TransactionKind{ID: kinds[dfltKindIdx].ID},
		ScheduledTo: time.Now(),
	}
}

func (h Handler) mapFDToTransactionFrequency(r *http.Request, parsedTx *models.Transaction) (*models.FrequentTx, error) {
	frequency, err := parseIntIfExists(r.FormValue("frequency"))
	if err != nil {
		return nil, err
	}

	frequencyBy, err := parseIntIfExists(r.FormValue("frequent_by"))
	if err != nil {
		return nil, err
	}

	stopsOn, err := parseDateOnlyIfExists(r.FormValue("stops_on"))
	if err != nil {
		return nil, err
	}

	return &models.FrequentTx{
		Amount:      parsedTx.Amount,
		Currency:    parsedTx.Currency,
		Kind:        parsedTx.Kind,
		Description: parsedTx.Description,
		FrequencyBy: models.FrequencyBy(frequencyBy),
		Frequency:   uint(frequency),
		StartsOn:    parsedTx.ScheduledTo,
		StopsOn:     stopsOn,
	}, nil
}
