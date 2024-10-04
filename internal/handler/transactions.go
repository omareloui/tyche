package handler

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	accs, knds, cats, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	fd := h.mapTransactionToFD(defaultTransaction(accs, knds), new(errs.ValidationError))

	transactions, err := h.repo.GetTransactions(r.Context())
	if err != nil {
		l.Error("fetch transaction from db", zap.Error(err))
		return nil, err
	}

	l.Debug("transaction from data", zap.Any("fd", fd), zap.Int("transactions_count", len(transactions)))

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cats = getCategoriesOfKinds(cats, fd.KindID.Value)

	return views.Transactions(balances, transactions, fd, accs, knds, cats), nil
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r)
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()

	sod := time.Now().Truncate(24 * time.Hour)
	if tx.ScheduledTo.Equal(sod) || tx.ScheduledTo.Before(sod) {
		tx.FulfilledOn = time.Now()
	}

	accs, knds, cats, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	fd, err := h.prepareTx(r, tx, accs, knds, cats)
	if fd != nil {
		return views.CreateTransactionForm(fd, accs, knds, cats), nil
	}
	if err != nil {
		return nil, err
	}

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
		ftx.LastTransactionCreatedAt = time.Now()

		if err := h.sani.SanitizeStruct(ftx); err != nil {
			return nil, errs.ErrSanitizer
		}

		if valerr := h.valid.Validate(ftx); valerr != nil {
			l.Error("frequent transaction validation", zap.Error(valerr), zap.Any("errors", valerr.Errors))
			fd := h.mapTransactionToFD(tx, valerr)

			fd.FrequencyFD = h.parseFrequencyFD(r, valerr)

			return views.CreateTransactionForm(fd, accs, knds, cats), nil
		}

		scheduledTxs = generateNextScheduledTransactions(ftx, generateNextScheduledTransactionsCount)
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

	fd = h.mapTransactionToFD(defaultTransaction(accs, knds), new(errs.ValidationError))
	return views.CreateTransactionForm(fd, accs, knds, cats), nil
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

	accs, knds, cats, err := h.getTxDeps(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))
	fd.FrequencyFD = nil

	return views.EditTransaction(tx, fd, accs, knds, cats), nil
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

	accs, knds, cats, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	fd, err := h.prepareTx(r, tx, accs, knds, cats)
	if fd != nil {
		return views.EditTransaction(tx, fd, accs, knds, cats), nil
	}
	if err != nil {
		return nil, err
	}

	l.Debug("update transaction", zap.Any("new_transaction", tx))

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

	tx.Kind = *txKind

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

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		l.Error("fetching transaction", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched transaction", zap.Any("tx", tx))

	tx.FulfilledOn = time.Now()

	if err := h.repo.FulfillTransaction(r.Context(), tx); err != nil {
		return nil, err
	}
	l.Info("fulfilled transaction")

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

	if err := h.repo.DeleteTransaction(r.Context(), tx); err != nil {
		l.Error("deleting transaction", zap.Error(err))
		return nil, err
	}
	l.Info("deleted transaction")

	if !tx.FulfilledOn.IsZero() {
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

func (h Handler) getTxDeps(ctx context.Context) ([]models.Account, []models.TransactionKind, []models.Category, error) {
	l := logger.FromCtx(ctx)
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		l.Error("error fetching accounts", zap.Error(err))
		return nil, nil, nil, err
	}
	knds, err := h.repo.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("error fetching transaction kinds", zap.Error(err))
		return nil, nil, nil, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		l.Error("error fetching categories", zap.Error(err))
		return nil, nil, nil, err
	}
	l.Debug("fetched transactions dependencies")
	return accs, knds, cats, nil
}

func (h Handler) mapTransactionToFD(t *models.Transaction, err *errs.ValidationError) *views.TransactionFD {
	fd := &views.TransactionFD{
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: views.AccountIFD{
				Amount:    views.FDInput{Value: formatFloatIfNonZero(t.Amount), Error: err.Errors.MsgFor("Amount")},
				AccountID: views.FDInput{Value: t.Account.ID, Error: err.Errors.MsgFor("Account.ID")},
			},
			KindID: views.FDInput{Value: t.Kind.ID, Error: err.Errors.MsgFor("Kind.ID")},
		},
		Description: views.FDInput{Value: t.Description, Error: err.Errors.MsgFor("Description")},
		ScheduledTo: views.FDInput{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
		FrequencyFD: &views.FrequencyFD{},
	}

	if t.Category != nil {
		fd.CategoryID = &views.FDInput{Value: t.Category.ID, Error: err.Errors.MsgFor("Category.ID")}
	}

	if t.ToAccount != nil {
		fd.TransferAccountIFD = &views.AccountIFD{
			Amount:    views.FDInput{Value: formatFloatIfNonZero(t.ToAmount), Error: err.Errors.MsgFor("ToAmount")},
			AccountID: views.FDInput{Value: t.ToAccount.ID, Error: err.Errors.MsgFor("ToAccount.ID")},
		}
	}

	return fd
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

	kndId := r.FormValue("kind_id")

	var cat *models.Category
	if catId := r.FormValue("category_id"); catId != "" {
		cat = &models.Category{ID: catId, Kind: models.TransactionKind{ID: kndId}}
	}

	return &models.Transaction{
		Account:     models.Account{ID: r.FormValue("account_id")},
		Amount:      amount,
		Kind:        models.TransactionKind{ID: kndId},
		Category:    cat,
		Description: r.FormValue("description"),
		ScheduledTo: scheduledTo,
	}, nil
}

func defaultTransaction(accs []models.Account, knds []models.TransactionKind) *models.Transaction {
	dfltKindIdx := slices.IndexFunc(knds, func(knd models.TransactionKind) bool { return knd.Kind == models.ExpenseTxKind })

	var dfltAccId string
	if len(accs) > 0 {
		dfltAcc := slicesutils.Find(accs, func(a models.Account) bool { return a.IsDefault })
		if dfltAcc != nil {
			dfltAccId = dfltAcc.ID
		} else {
			dfltAccId = accs[0].ID
		}
	}

	knd := knds[dfltKindIdx]

	tx := &models.Transaction{
		Amount:      0,
		Account:     models.Account{ID: dfltAccId},
		Kind:        knd,
		ScheduledTo: time.Now(),
	}

	if knd.Kind == models.ExpenseTxKind {
		tx.Category = &models.Category{}
	}

	return tx
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
		Account:     parsedTx.Account,
		Amount:      parsedTx.Amount,
		Kind:        parsedTx.Kind,
		Category:    parsedTx.Category,
		Description: parsedTx.Description,
		FrequencyBy: models.FrequencyBy(frequencyBy),
		Frequency:   uint(frequency),
		StartsOn:    parsedTx.ScheduledTo,
		StopsOn:     stopsOn,
	}, nil
}

func (h Handler) prepareTx(r *http.Request, tx *models.Transaction, accs []models.Account, knds []models.TransactionKind, cats []models.Category) (*views.TransactionFD, error) {
	l := logger.FromCtx(r.Context())

	k := slicesutils.Find(knds, func(k models.TransactionKind) bool { return k.ID == tx.Kind.ID })
	if k == nil {
		valerr := &errs.ValidationError{Errors: errs.ValidationErrors{}}
		valerr.Errors["Kind.ID"] = errs.ValidationField{Tag: "required"}
		l.Error("tx kind doesn't exist")
		fd := h.mapTransactionToFD(tx, valerr)
		fd.FrequencyFD = h.parseFrequencyFD(r, valerr)
		return fd, nil
	}
	tx.Kind = *k

	a := slicesutils.Find(accs, func(ac models.Account) bool { return ac.ID == tx.Account.ID })
	if a == nil {
		valerr := &errs.ValidationError{Errors: errs.ValidationErrors{}}
		valerr.Errors["Account.ID"] = errs.ValidationField{Tag: "required"}
		l.Error("account doesn't exist")
		fd := h.mapTransactionToFD(tx, valerr)
		fd.FrequencyFD = h.parseFrequencyFD(r, valerr)
		return fd, nil
	}
	tx.Account = *a

	if c := slicesutils.Find(cats, func(c models.Category) bool { return tx.Category != nil && c.ID == tx.Category.ID }); c != nil {
		tx.Category = c
		tx.Category.Kind = tx.Kind
	}

	if err := h.sani.SanitizeStruct(tx); err != nil {
		return nil, errs.ErrSanitizer
	}

	if valerr := h.valid.Validate(tx); valerr != nil {
		l.Error("validating transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
		fd := h.mapTransactionToFD(tx, valerr)
		return fd, nil
	}

	return nil, nil
}

func (h Handler) prepareCreatingTx(r *http.Request, tx *models.Transaction, accs []models.Account, knds []models.TransactionKind, cats []models.Category) (*views.TransactionFD, error) {
	tx.ID = idgen.New()
	tx.CreatedAt = time.Now()

	sod := time.Now().Truncate(24 * time.Hour)
	if tx.ScheduledTo.Equal(sod) || tx.ScheduledTo.Before(sod) {
		tx.FulfilledOn = tx.CreatedAt
	}

	return h.prepareTx(r, tx, accs, knds, cats)
}

func getCategoriesOfKinds(cats []models.Category, kndId string) []models.Category {
	kndsCats := []models.Category{}
	for _, cat := range cats {
		if cat.Kind.ID == kndId {
			kndsCats = append(kndsCats, cat)
		}
	}
	return kndsCats
}
