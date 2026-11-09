package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) Transactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	list, err := h.txListData(r, models.ParseTxFilter(r.URL.Query()), time.Time{}, nil)
	if err != nil {
		return nil, err
	}

	// The filters form swaps only the list, so typing in it isn't disturbed.
	target := r.Header.Get("HX-Target")
	if target == "tx-list" {
		return views.TxList(*list), nil
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	gls, err := h.repo.GetGoals(r.Context())
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	if target == "tx-body" {
		return views.TxBody(*list, accs, knds, cats, cps, gls), nil
	}

	fd := h.mapTransactionToFD(defaultTransaction(accs), new(errs.ValidationError))

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched balances")

	// ?tx= opens the transaction's details, e.g. when reloading the page.
	openTx := r.URL.Query().Get(views.TxDetailsParam)

	return views.Transactions(getClaims(r.Context()), balances, *list, fd, accs, knds, cats, cps, gls, openTx), nil
}

// TransactionsMore is the chunk of the list older than ?before=.
func (h Handler) TransactionsMore(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	before, err := time.Parse(time.DateOnly, r.URL.Query().Get("before"))
	if err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid before date")
	}
	list, err := h.txListData(r, models.ParseTxFilter(r.URL.Query()), before, nil)
	if err != nil {
		return nil, err
	}
	return views.TxListChunk(*list), nil
}

func (h Handler) txListData(r *http.Request, f models.TxFilter, before time.Time, until *time.Time) (*views.TxListData, error) {
	page, err := h.repo.GetTransactionsPage(r.Context(), f, before, until)
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetch transactions from db", zap.Error(err))
		return nil, err
	}
	d := &views.TxListData{Page: page, Filter: f, Def: &getClaims(r.Context()).DefaultCurrency}
	// The upcoming groups stay as the page has them (transactions.js).
	d.UpcomingOpen = openHeader(r, "X-Tx-Upcoming")
	d.LaterOpen = openHeader(r, "X-Tx-Upcoming-Later")
	// Only the first chunk (or a re-render) shows the summary.
	if !f.IsZero() && before.IsZero() {
		if d.Summary, err = h.repo.GetTransactionsSummary(r.Context(), f, d.Def); err != nil {
			return nil, err
		}
		d.Chips = h.txFilterChips(r.Context(), &f)
	}
	return d, nil
}

// txFilterChips are the filters set, each removable on its own.
func (h Handler) txFilterChips(ctx context.Context, f *models.TxFilter) []views.TxFilterChip {
	chips := []views.TxFilterChip{}
	add := func(label string, keys ...string) {
		chips = append(chips, views.TxFilterChip{Label: label, Without: f.Without(keys...)})
	}
	if f.Q != "" {
		add("“"+f.Q+"”", models.TxFilterQ)
	}
	if f.Kind != models.NilTxKind {
		add(f.Kind.String(), models.TxFilterKind)
	}
	// The ids' names, "Unknown" when gone.
	if f.AccountID != "" {
		label := "Unknown"
		if acc, err := h.repo.GetAccount(ctx, f.AccountID); err == nil {
			label = acc.Name
		}
		add(label, models.TxFilterAccount)
	}
	if f.CategoryID == models.TxFilterNone {
		add("Uncategorized", models.TxFilterCategory)
	} else if f.CategoryID != "" {
		label := "Unknown"
		if cat, err := h.repo.GetCategory(ctx, f.CategoryID); err == nil {
			label = cat.Name
		}
		add(label, models.TxFilterCategory)
	}
	if f.CounterpartyID == models.TxFilterNone {
		add("No counterparty", models.TxFilterCounterparty)
	} else if f.CounterpartyID != "" {
		label := "Unknown"
		if cp, err := h.repo.GetCounterparty(ctx, f.CounterpartyID); err == nil {
			label = cp.Name
		}
		add(label, models.TxFilterCounterparty)
	}
	if f.GoalID != "" {
		label := "Unknown"
		if gl, err := h.repo.GetGoal(ctx, f.GoalID); err == nil {
			label = gl.Name
		}
		add(label, models.TxFilterGoal)
	}
	switch f.Status {
	case models.TxStatusFulfilled:
		add("Fulfilled", models.TxFilterStatus)
	case models.TxStatusScheduled:
		add("Scheduled", models.TxFilterStatus)
	case models.TxStatusReview:
		add("Needs review", models.TxFilterStatus)
	}
	switch f.Source {
	case models.TxSourceSMS:
		add("From SMS", models.TxFilterSource)
	case models.TxSourceManual:
		add("Added by hand", models.TxFilterSource)
	}
	switch f.Receipt {
	case models.TxReceiptWith:
		add("With a receipt", models.TxFilterReceipt)
	case models.TxReceiptWithout:
		add("Without a receipt", models.TxFilterReceipt)
	}
	if !f.From.IsZero() || !f.To.IsZero() {
		add(views.FormatTxRange(f.From, f.To), models.TxFilterFrom, models.TxFilterTo)
	}
	if f.MinAmount != nil || f.MaxAmount != nil {
		add(views.FormatAmountRange(f.MinAmount, f.MaxAmount), models.TxFilterMin, models.TxFilterMax)
	}
	return chips
}

// loadedTxListData is the list as the page shows it, to re-render it after
// changing a transaction: filtered by the page's query string, loaded back
// to the X-Tx-Until header (transactions.js).
func (h Handler) loadedTxListData(r *http.Request) (*views.TxListData, error) {
	var f models.TxFilter
	if u, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && u.Path == "/transactions" {
		f = models.ParseTxFilter(u.Query())
	}

	var until *time.Time
	switch v := r.Header.Get("X-Tx-Until"); v {
	case "":
	case "all":
		until = new(time.Time)
	default:
		if d, err := time.Parse(time.DateOnly, v); err == nil {
			until = &d
		}
	}
	return h.txListData(r, f, time.Time{}, until)
}

func (h Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	tx, err := h.mapFDToTransaction(r, r.FormValue("is_recurrent") != "")
	if err != nil {
		return nil, err
	}
	tx.ID = idgen.New()

	if !tx.ScheduledTo.After(h.userToday(r.Context())) {
		tx.FulfilledAt = time.Now()
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	applyCounterpartyDefaultCategory(tx, cps)

	claims := getClaims(r.Context())

	// Set when buying a wishlist item, which the transaction then marks bought.
	wishlistID := r.FormValue("wishlist_id")

	fd, err := h.prepareTx(r, tx, accs, knds, cats, cps)
	if fd != nil {
		fd.WishlistID = wishlistID
		return views.CreateTransactionForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
	}
	if err != nil {
		return nil, err
	}

	var scheduledTxs []models.Transaction

	if tx.RecurrentTx != nil {
		tx.RecurrentTx.ID = idgen.New()

		if err := tx.RecurrentTx.SanitizeAndValidate(h.sani, h.valid); err != nil {
			var valerr errs.ValidationError
			if errors.As(err, &valerr) {
				l.Error("validating recurrent transaction", zap.Error(valerr), zap.Any("errors", valerr.Errors))
				fd := h.mapTransactionToFD(tx, &valerr)
				fd.WishlistID = wishlistID
				return views.CreateTransactionForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
			}
			l.Error("sanitizing recurrent transaction", zap.Error(err))
			return nil, err
		}
	}

	if wishlistID != "" && tx.RecurrentTx == nil {
		item, err := h.repo.GetWishlistItem(r.Context(), wishlistID)
		if err != nil {
			return nil, err
		}
		scheduledTxs = append(scheduledTxs, wishlistPlanTxs(item, tx)...)
	}

	if err := h.repo.CreateTransaction(r.Context(), tx, tx.RecurrentTx, scheduledTxs); err != nil {
		l.Error("creating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("created transaction")
	drawerCreated(w)

	if wishlistID != "" {
		if err := h.repo.SetWishlistItemStatus(r.Context(), wishlistID, models.WishlistBought, tx.ID); err != nil {
			return nil, err
		}
		hxRespondWithRedirect(w, "/goals?tab="+views.GoalsTabWishlist)
		return nil, nil
	}

	// Re-render the whole list so new transactions land in their day group.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}

	if !tx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	fd = h.mapTransactionToFD(defaultTransaction(accs), new(errs.ValidationError))
	return views.CreateTransactionForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
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

	if tx.Kind == models.AdjustmentTxKind {
		return nil, errAdjustmentNotEditable
	}
	if tx.IsLocked() {
		return nil, errTxLocked
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		l.Error("fetching currencies and kinds")
		return nil, err
	}

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))
	fd.RecurrenceFD = nil

	def := &getClaims(r.Context()).DefaultCurrency
	// From the open details the form swaps in their place, else (a row's
	// menu, "e") it opens the drawer.
	if r.Header.Get("HX-Target") == txDetailsPanel {
		return views.TxEditPanel(tx, fd, accs, knds, cats, cps, def, views.TxEditFromDetails), nil
	}
	return views.TxEditDrawer(tx, fd, accs, knds, cats, cps, def), nil
}

// txEditFrom is where the edit form posted was opened from.
func txEditFrom(r *http.Request) string {
	if r.FormValue("from") == views.TxEditFromRow {
		return views.TxEditFromRow
	}
	return views.TxEditFromDetails
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

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}
	def := &getClaims(r.Context()).DefaultCurrency
	fd, err := h.prepareTx(r, tx, accs, knds, cats, cps)
	if fd != nil {
		return views.TxEditPanel(tx, fd, accs, knds, cats, cps, def, txEditFrom(r)), nil
	}
	if err != nil {
		return nil, err
	}

	oldTx, err := h.repo.GetTransaction(r.Context(), tx.ID)
	if err != nil {
		return nil, err
	}
	if oldTx.IsLocked() {
		return nil, errTxLocked
	}

	tx.FulfilledAt = oldTx.FulfilledAt
	tx.CreatedAt = oldTx.CreatedAt
	// The form only edits the date, keep the original time of day so the
	// transaction keeps its order within the day.
	tx.ScheduledTo = models.KeepTimeOfDay(tx.ScheduledTo, oldTx.ScheduledTo)

	if err := h.repo.UpdateTransaction(r.Context(), oldTx, tx); err != nil {
		var valerr *errs.ValidationError
		if errors.As(err, &valerr) {
			fd := h.mapTransactionToFD(tx, valerr)
			fd.RecurrenceFD = nil
			return views.TxEditPanel(tx, fd, accs, knds, cats, cps, def, txEditFrom(r)), nil
		}
		l.Error("updating transaction", zap.Error(err))
		return nil, err
	}
	l.Info("updated transaction")

	// The drawer shows the saved transaction (or closes, on drawer-saved, when
	// opened from the row); the list behind it re-renders out of band.
	d, err := h.txDetails(r.Context(), id)
	if err != nil {
		return nil, err
	}
	addHXTrigger(w, drawerSavedEvent)
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	if !oldTx.FulfilledAt.IsZero() {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return views.TxDetailsPanel(d, def), nil
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

	tx.FulfilledAt = time.Now()

	if err := h.repo.FulfillTransaction(r.Context(), tx); err != nil {
		return nil, err
	}
	l.Info("fulfilled transaction")

	hxRetargetTxList(w)

	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}

	return h.transactionsList(r)
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

	hxRetargetTxList(w)
	return h.transactionsList(r)
}

// MarkTransactionNeedsReview sends a transaction back to review.
func (h Handler) MarkTransactionNeedsReview(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	if err := h.repo.MarkTransactionNeedsReview(r.Context(), id); err != nil {
		return nil, err
	}
	l.Info("marked transaction as needing review")

	hxRetargetTxList(w)
	return h.transactionsList(r)
}

func (h Handler) MarkAllTransactionsReviewed(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	if err := h.repo.MarkAllTransactionsReviewed(r.Context()); err != nil {
		return nil, err
	}
	l.Info("marked all transactions as reviewed")

	hxRetargetTxList(w)
	return h.transactionsList(r)
}

// ReviewTransactionCategory sets the category of a transaction under review,
// marking it as reviewed.
func (h Handler) ReviewTransactionCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	catID := r.FormValue("category_id")
	if catID != "" {
		cats, err := h.repo.GetCategories(r.Context())
		if err != nil {
			l.Error("fetching categories", zap.Error(err))
			return nil, err
		}
		cat := slicesutils.Find(cats, func(c models.Category) bool { return c.ID == catID })
		if cat == nil || !slices.Contains(cat.Kinds, tx.Kind.CategoryKind()) {
			l.Error("category doesn't apply to the transaction", zap.String("category_id", catID))
			return nil, errs.ErrInvalidID
		}
	}

	if err := h.repo.ReviewTransactionCategory(r.Context(), id, catID); err != nil {
		return nil, err
	}
	l.Info("reviewed transaction category", zap.String("category_id", catID))

	hxRetargetTxList(w)
	return h.transactionsList(r)
}

func (h Handler) GetReviewCategoryPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		return nil, err
	}

	return views.TxReviewCategoryPicker(tx, cats), nil
}

// DuplicateTransaction answers the create form filled with the transaction,
// dated today.
func (h Handler) DuplicateTransaction(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	if tx.Kind == models.AdjustmentTxKind {
		return nil, errAdjustmentNotEditable
	}
	if tx.IsLocked() {
		return nil, errTxLocked
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	dup := *tx
	dup.ScheduledTo = time.Now()
	dup.FulfilledAt = time.Time{}
	dup.RecurrentTx = nil
	dup.Items = models.CopyTxItems(tx.Items)
	dup.Receipt, dup.ReceiptID = nil, ""

	claims := getClaims(r.Context())
	fd := h.mapTransactionToFD(&dup, new(errs.ValidationError))
	return views.CreateTransactionForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency), nil
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

	return h.deleteTransactions(w, r, []*models.Transaction{tx})
}

// DeleteTransactions deletes the selected transactions at once (bulkTxs):
// the ones waiting out the undo toast (transactions.js), or every one the
// filter matches. Ids already gone are skipped.
func (h Handler) DeleteTransactions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	txs, err := h.bulkTxs(r)
	if err != nil {
		return nil, err
	}
	// Goals' purchases are undone from their goal: left out of a selection
	// (shown back as the list re-renders), refused alone.
	if n := len(txs); n > 1 {
		txs = slices.DeleteFunc(txs, func(tx *models.Transaction) bool { return tx.IsGoalPurchase() && tx.Goal != nil })
		if len(txs) == 0 {
			return nil, errGoalPurchaseDelete
		}
	}
	return h.deleteTransactions(w, r, txs)
}

// deleteTransactions deletes txs, answering the list (and the balances when
// one was fulfilled).
func (h Handler) deleteTransactions(w http.ResponseWriter, r *http.Request, txs []*models.Transaction) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	fulfilled := false
	for _, tx := range txs {
		if tx.IsGoalPurchase() && tx.Goal != nil {
			return nil, errGoalPurchaseDelete
		}
		fulfilled = fulfilled || !tx.FulfilledAt.IsZero()
	}

	if err := h.repo.DeleteTransactions(r.Context(), txs); err != nil {
		l.Error("deleting transactions", zap.Error(err))
		return nil, err
	}
	l.Info("deleted transactions", zap.Int("count", len(txs)))

	hxRetargetTxList(w)

	if fulfilled {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return h.transactionsList(r)
}

// hxRetargetTxList swaps the response in place of the whole transactions
// list, as changing a transaction moves its totals, sections and counts. Call
// it before rendering anything, as it sets headers.
// errAdjustmentNotEditable is returned for forms of adjustments, as their
// amount is signed; they come from fixing balances.
var errAdjustmentNotEditable = resperr.New(http.StatusUnprocessableEntity, "Balance corrections can't be edited, fix the balance again instead.")

// errTxLocked is returned for forms of a goal's transactions (made by the
// goal's actions) and of a deleted account's.
var errTxLocked = resperr.New(http.StatusUnprocessableEntity, "Transactions of goals and deleted accounts can't be edited, delete it instead.")

// errGoalPurchaseDelete is returned deleting a goal's purchase, undone from
// the goal.
var errGoalPurchaseDelete = resperr.New(http.StatusUnprocessableEntity, "It's a goal's purchase, undo buying it from the goal instead.")

func hxRetargetTxList(w http.ResponseWriter) {
	w.Header().Set("HX-Retarget", "#tx-list")
	w.Header().Set("HX-Reswap", "outerHTML")
}

func (h Handler) transactionsList(r *http.Request) (templ.Component, error) {
	list, err := h.loadedTxListData(r)
	if err != nil {
		return nil, err
	}
	return views.TxList(*list), nil
}

func (h Handler) renderTransactionsListOOB(w http.ResponseWriter, r *http.Request) error {
	l := logger.FromCtx(r.Context())
	list, err := h.loadedTxListData(r)
	if err != nil {
		return err
	}
	if err = renderToBody(w, r, views.TransactionsListOOB(*list)); err != nil {
		l.Error("rendering transactions list OOB", zap.Error(err))
		return err
	}
	l.Debug("rendered transactions list OOB")
	return nil
}

func (h Handler) getAndRenderBalancesOOB(w http.ResponseWriter, r *http.Request) error {
	l := logger.FromCtx(r.Context())
	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return err
	}

	if err = renderToBody(w, r, views.BalanceOOB(balances, &getClaims(r.Context()).DefaultCurrency)); err != nil {
		l.Error("rendering balance OOB")
		return err
	}
	l.Debug("rendered balance OOB")
	return nil
}

func (h Handler) getTxDeps(ctx context.Context) ([]models.Account, []models.TxKind, []models.Category, []models.Counterparty, error) {
	l := logger.FromCtx(ctx)
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		l.Error("error fetching accounts for dx deps", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		l.Error("error fetching categories for dx deps", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		l.Error("error fetching counterparties for dx deps", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	l.Debug("fetched transactions dependencies")
	return accs, models.GetTxKinds(), cats, cps, nil
}

func (h Handler) mapTransactionToFD(t *models.Transaction, err *errs.ValidationError) *views.TransactionFD {
	fd := &views.TransactionFD{
		TransactionAmountsAndKindFD: views.TransactionAmountsAndKindFD{
			AccountIFD: amountIFD(t.Amount, &t.Account, form.FDInput{Value: t.Account.ID, Error: err.Errors.MsgFor("AccountID")},
				err.Errors.MsgFor("Amount")),
			Kind: form.FDInput{Value: t.Kind.String(), Error: err.Errors.MsgFor("Kind")},
		},
		Description:  form.FDInput{Value: t.Description, Error: err.Errors.MsgFor("Description")},
		ScheduledTo:  form.FDInput{Value: formatDateOnlyIfNonZero(t.ScheduledTo), Error: err.Errors.MsgFor("ScheduledTo")},
		RecurrenceFD: &views.RecurrenceFD{},
		Items:        txItemsToFD(t.Items, err),
		Receipt:      views.ReceiptFD{Enabled: h.ocr != nil, ID: t.ReceiptID},
	}
	if t.ReceiptID != "" {
		fd.Receipt.URL = (&models.Receipt{ID: t.ReceiptID, Hash: "new"}).URL()
	}

	hasToAccount := t.Kind == models.DebtTxKind || t.Kind == models.TransferTxKind

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
		toIFD := amountIFD(t.ToAmount, t.ToAccount, form.FDInput{Value: t.ToAccount.ID, Error: err.Errors.MsgFor("ToAccountID")},
			err.Errors.MsgFor("ToAmount"))
		fd.ToAccountIFD = &toIFD
	} else if hasToAccount {
		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Error: err.Errors.MsgFor("ToAmount")},
			AccountID: form.FDInput{Error: err.Errors.MsgFor("ToAccountID")},
		}
	}

	if t.RecurrentTx != nil {
		fd.RecurrenceFD = &views.RecurrenceFD{
			IsRecurrent: form.FDInput{Value: "on"},
			Unit:        form.FDInput{Value: formatIntIfNonZero(int(t.RecurrentTx.Unit)), Error: err.Errors.MsgFor("Unit")},
			Every:       form.FDInput{Value: formatIntIfNonZero(int(t.RecurrentTx.Every)), Error: err.Errors.MsgFor("Every")},
			StopsOn:     form.FDInput{Value: formatDateOnlyIfNonZero(t.RecurrentTx.StopsAt), Error: err.Errors.MsgFor("StopsAt")},
			AutoFulfill: form.FDInput{Value: onIf(t.RecurrentTx.AutoFulfill)},
		}
	}

	return fd
}

// onIf is a checkbox's value: "on" when checked.
func onIf(checked bool) string {
	if checked {
		return "on"
	}
	return ""
}

// amountIFD is the inputs of an amount of acc. A metal account's amount is
// in the unit its currency is shown in; when the currency isn't loaded (the
// form answered with its errors) it stays in troy ounces, as parsed.
func amountIFD(amount float64, acc *models.Account, accID form.FDInput, amountErr string) form.AccountIFD {
	ifd := form.AccountIFD{
		Amount:    form.FDInput{Value: formatFloatIfNonZero(amount), Error: amountErr},
		AccountID: accID,
		Unit:      form.FDInput{Value: string(models.MetalUnitOunce)},
	}
	if acc != nil && acc.Currency.IsMetal() {
		u := acc.Currency.Unit()
		ifd.Amount.Value = models.InputValue(amount, u)
		ifd.Unit.Value = string(u)
	}
	return ifd
}

func (h Handler) mapFDToTransaction(r *http.Request, isRecurrent bool) (*models.Transaction, error) {
	amount, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}
	amount = models.FromUnit(amount, models.ParseMetalUnit(r.FormValue("amount_unit")))

	toAmount, err := parseFloatIfExists(r.FormValue("to_amount"))
	if err != nil {
		return nil, err
	}
	toAmount = models.FromUnit(toAmount, models.ParseMetalUnit(r.FormValue("to_amount_unit")))

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

	tx := &models.Transaction{
		Account:      models.Account{ID: r.FormValue("account_id")},
		Amount:       amount,
		Kind:         knd,
		Category:     cat,
		Counterparty: cp,
		ToAccount:    toAcc,
		ToAmount:     toAmount,
		Description:  r.FormValue("description"),
		ScheduledTo:  scheduledTo,
		Items:        parseTxItems(r),
		ReceiptID:    strings.TrimSpace(r.FormValue("receipt_id")),
	}

	if isRecurrent {
		tx.RecurrentTx, err = h.mapFDAndTxToRecurrentTx(r, tx)
		l := logger.FromCtx(r.Context())
		if err != nil {
			l.Error("parsing recurrent transaction", zap.Error(err))
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
		Unit:         models.RecurrentUnit(unit),
		Every:        uint(every),
		StartsAt:     tx.ScheduledTo,
		StopsAt:      stopsOn,
		// tx is its first occurrence, the next ones counted from it.
		Anchor:      tx.ScheduledTo,
		NextIndex:   1,
		AutoFulfill: r.FormValue("auto_fulfill") != "",
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
	normalizeTxItems(tx)

	if c := slicesutils.Find(cats, func(c models.Category) bool { return tx.Category != nil && c.ID == tx.Category.ID }); c != nil {
		tx.Category = c
		tx.Category.Kinds = []models.TxKind{tx.Kind.CategoryKind()}
	}

	if cp := slicesutils.Find(cps, func(c models.Counterparty) bool { return tx.Counterparty != nil && c.ID == tx.Counterparty.ID }); cp != nil {
		tx.Counterparty = cp
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
) (*views.TransactionFD, error) {
	tx.ID = idgen.New()
	tx.CreatedAt = time.Now()

	now := time.Now()
	if tx.ScheduledTo.Equal(now) || tx.ScheduledTo.Before(now) {
		tx.FulfilledAt = tx.CreatedAt
	}

	return h.prepareTx(r, tx, accs, knds, cats, cps)
}

func (h Handler) parseTransactionAmountsAndKindFD(r *http.Request, accs []models.Account, knds []models.TxKind) (*views.TransactionAmountsAndKindFD, error) {
	accId := r.FormValue("account_id")
	kindString := r.FormValue("kind")

	fd := &views.TransactionAmountsAndKindFD{
		AccountIFD: form.AccountIFD{
			Amount:    form.FDInput{Value: r.FormValue("amount")},
			AccountID: form.FDInput{Value: accId},
			Unit:      form.FDInput{Value: r.FormValue("amount_unit")},
		},
		Kind: form.FDInput{Value: kindString},
	}

	trigger := r.Header.Get("Hx-Trigger")

	kind := models.GetTxKindFromString(kindString)

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
			amt = models.FromUnit(amt, acc.AmountUnit(fd.AccountIFD.Unit.Value))
			toU := toAcc.AmountUnit(r.FormValue("to_amount_unit"))
			toAmt = strconv.FormatFloat(models.ToUnit(amt/fxRate, toU), 'f', models.AmountDigits(toU), 64)
		}

		fd.ToAccountIFD = &form.AccountIFD{
			Amount:    form.FDInput{Value: toAmt},
			AccountID: form.FDInput{Value: toAccId},
			Unit:      form.FDInput{Value: r.FormValue("to_amount_unit")},
		}
	} else {
		fd.CategoryID = &form.FDInput{Value: r.FormValue("category_id")}
		fd.CounterpartyID = &form.FDInput{Value: r.FormValue("counterparty_id")}
	}

	return fd, nil
}

// openHeader is whether the page has a folding section open, from its
// header (open / closed), nil when it doesn't tell.
func openHeader(r *http.Request, name string) *bool {
	var open bool
	switch r.Header.Get(name) {
	case "open":
		open = true
	case "closed":
	default:
		return nil
	}
	return &open
}
