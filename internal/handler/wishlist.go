package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/components"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// Wishlist renders the whole wishlist, which every change on it re-renders.
func (h Handler) wishlist(r *http.Request, oob bool) (templ.Component, error) {
	items, err := h.repo.GetWishlistItems(r.Context())
	if err != nil {
		return nil, err
	}

	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	return views.Wishlist(items, ws, &claims.DefaultCurrency, oob), nil
}

func (h Handler) CreateWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	item, err := mapFDToWishlistItem(r, curs)
	if err != nil {
		l.Error("parsing form to wishlist item", zap.Error(err))
		return nil, err
	}
	item.ID = idgen.New()
	item.CreatedAt = time.Now()

	claims := getClaims(r.Context())

	if err := item.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			return views.CreateWishlistItemForm(mapWishlistItemToFD(item, &valerr), curs, &claims.DefaultCurrency), nil
		}
		return nil, err
	}

	if err := h.repo.CreateWishlistItem(r.Context(), item); err != nil {
		l.Error("creating wishlist item", zap.Error(err))
		return nil, err
	}
	l.Info("created wishlist item", zap.String("id", item.ID))
	drawerCreated(w)

	list, err := h.wishlist(r, true)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, list); err != nil {
		return nil, err
	}

	return views.CreateWishlistItemForm(mapWishlistItemToFD(dfltWishlistItem(&claims.DefaultCurrency), &errs.ValidationError{}), curs, &claims.DefaultCurrency), nil
}

func (h Handler) GetEditWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	item, err := h.repo.GetWishlistItem(r.Context(), id)
	if err != nil {
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	fd := mapWishlistItemToFD(item, &errs.ValidationError{})
	fd.From = wishFrom(r)
	return views.EditWishlistItem(id, fd, curs, &claims.DefaultCurrency), nil
}

func (h Handler) EditWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	item, err := mapFDToWishlistItem(r, curs)
	if err != nil {
		return nil, err
	}
	item.ID = id

	if err := item.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			claims := getClaims(r.Context())
			fd := mapWishlistItemToFD(item, &valerr)
			fd.From = wishFrom(r)
			return views.EditWishlistItemForm(id, fd, curs, &claims.DefaultCurrency), nil
		}
		return nil, err
	}

	if err := h.repo.UpdateWishlistItem(r.Context(), item); err != nil {
		l.Error("updating wishlist item", zap.Error(err))
		return nil, err
	}
	l.Info("updated wishlist item", zap.String("id", id))
	if wishFrom(r) != "" {
		return h.wishAnswer(w, r, id)
	}
	drawerSaved(w, "#wishlist", "outerHTML")

	return h.wishlist(r, false)
}

// WishlistItemDetails is the wish in full, in a drawer: its price, cooling
// off, actions, link and note.
func (h Handler) WishlistItemDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	return h.wishDetails(r, id)
}

func (h Handler) wishDetails(r *http.Request, id string) (templ.Component, error) {
	item, err := h.repo.GetWishlistItem(r.Context(), id)
	if err != nil {
		return nil, err
	}
	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}
	claims := getClaims(r.Context())
	return views.WishDetails(item, ws, &claims.DefaultCurrency), nil
}

// wishFrom is where the request's wish action or form was opened from:
// views.WishFromDetails, or "" (the card).
func wishFrom(r *http.Request) string {
	if r.FormValue("from") == views.WishFromDetails {
		return views.WishFromDetails
	}
	return ""
}

// wishAnswer answers a wish's action opened from its details: them again in
// the drawer, the wishlist re-rendered out of band. Like every header,
// before writing the body.
func (h Handler) wishAnswer(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	w.Header().Set("HX-Retarget", "#"+components.DrawerSlot)
	w.Header().Set("HX-Reswap", "innerHTML")
	list, err := h.wishlist(r, true)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, list); err != nil {
		return nil, err
	}
	return h.wishDetails(r, id)
}

func (h Handler) AbandonWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setWishlistItemStatus(w, r, models.WishlistAbandoned)
}

// RestoreWishlistItem puts a decided item back to waiting.
func (h Handler) RestoreWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setWishlistItemStatus(w, r, models.WishlistWaiting)
}

func (h Handler) setWishlistItemStatus(w http.ResponseWriter, r *http.Request, status models.WishlistStatus) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.SetWishlistItemStatus(r.Context(), id, status, ""); err != nil {
		return nil, err
	}
	if wishFrom(r) != "" {
		return h.wishAnswer(w, r, id)
	}
	return h.wishlist(r, false)
}

func (h Handler) DeleteWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteWishlistItem(r.Context(), id); err != nil {
		l.Error("deleting wishlist item", zap.String("id", id), zap.Error(err))
		return nil, err
	}
	l.Info("deleted wishlist item", zap.String("id", id))
	if wishFrom(r) != "" {
		// From its details: closing them.
		addHXTrigger(w, drawerSavedEvent)
	}

	return h.wishlist(r, false)
}

// BuyWishlistItem opens the transaction form prefilled from the item: its
// price, or its installments as a monthly recurrence. Creating the
// transaction marks the item bought.
func (h Handler) BuyWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	item, err := h.repo.GetWishlistItem(r.Context(), id)
	if err != nil {
		return nil, err
	}

	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	tx := defaultTransaction(accs)
	// Pay from an account in the item's currency when the default one isn't.
	if tx.Account.Currency.String != item.Currency.String {
		if acc := slicesutils.Find(accs, func(a models.Account) bool {
			return !a.IsArchive && a.Currency.String == item.Currency.String
		}); acc != nil {
			tx.Account = *acc
		}
	}

	// The form is the first payment, on the day or the plan's first month;
	// creating it schedules the plan's other payments.
	rate := wishlistFxRate(item, &tx.Account)
	tx.Amount = item.Price * rate
	tx.Description = item.Name
	if s := item.Schedule(); s != nil {
		if s.PaidOnDay() > 0 {
			tx.Amount = s.PaidOnDay() * rate
			tx.Description = item.Name + " (down payment)"
		} else {
			tx.Amount = s.FirstPayment() * rate
			tx.ScheduledTo = time.Now().AddDate(0, s.Rows[0].Month, 0)
			tx.Description = fmt.Sprintf("%s (1/%d)", item.Name, len(s.Rows))
		}
	}
	tx.Amount = models.RoundCents(tx.Amount)
	applyCounterpartyDefaultCategory(tx, cps)

	fd := h.mapTransactionToFD(r.Context(), tx, new(errs.ValidationError))
	fd.WishlistID = item.ID

	claims := getClaims(r.Context())
	return views.WishlistBuySheet(views.CreateTransactionForm(fd, accs, knds, cats, cps, &claims.DefaultCurrency)), nil
}

// GetWishGoal is the goal form saving for a waiting wish, prefilled from it.
func (h Handler) GetWishGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	item, err := h.waitingWish(r)
	if err != nil {
		return nil, err
	}
	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}
	gl := &models.Goal{Name: item.Name, Price: item.Price, Currency: item.Currency}
	claims := getClaims(r.Context())
	return views.WishGoal(item, h.mapGlToFD(gl, &errs.ValidationError{}), curs, &claims.DefaultCurrency), nil
}

// SaveWishForGoal creates the goal saving for the wish. A wish paid in
// installments goes on to the goal's plan, its loan prefilled.
func (h Handler) SaveWishForGoal(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	item, err := h.waitingWish(r)
	if err != nil {
		return nil, err
	}
	gl, err := h.mapFDToGoal(r)
	if err != nil {
		return nil, err
	}
	gl.ID = idgen.New()
	gl.CreatedAt = time.Now()

	renderForm := func(valerr *errs.ValidationError) (templ.Component, error) {
		curs, err := h.repo.GetCurrencies(r.Context())
		if err != nil {
			return nil, err
		}
		claims := getClaims(r.Context())
		return views.WishGoalForm(item, h.mapGlToFD(gl, valerr), curs, &claims.DefaultCurrency), nil
	}
	if err := gl.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			return renderForm(&valerr)
		}
		return nil, err
	}
	if err := h.repo.CreateGoalFromWish(r.Context(), gl, item.ID); err != nil {
		var valerr *errs.ValidationError
		if errors.As(err, &valerr) {
			return renderForm(valerr)
		}
		l.Error("creating goal from wish", zap.Error(err), zap.String("wish", item.ID))
		return nil, err
	}
	l.Info("created goal from wish", zap.String("goal", gl.ID), zap.String("wish", item.ID))

	if gl, err = h.repo.GetGoal(r.Context(), gl.ID); err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, views.GoalOOB(gl, nil)); err != nil {
		return nil, err
	}
	if err := h.renderGoalsSummaryOOB(w, r); err != nil {
		return nil, err
	}

	if item.Plan == nil {
		drawerSaved(w, "#wishlist", "outerHTML")
		return h.wishlist(r, false)
	}
	list, err := h.wishlist(r, true)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, list); err != nil {
		return nil, err
	}
	// The plan's drawer in place of this one.
	w.Header().Set("HX-Retarget", "#"+components.DrawerSlot)
	w.Header().Set("HX-Reswap", "innerHTML")
	r.SetPathValue("id", gl.ID)
	return h.GetGoalPlanForm(w, r)
}

// waitingWish is the request's wish, only while waiting (a decided one is
// a 409).
func (h Handler) waitingWish(r *http.Request) (*models.WishlistItem, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	item, err := h.repo.GetWishlistItem(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if item.Status != models.WishlistWaiting {
		return nil, resperr.New(http.StatusConflict, "This wish is already decided.")
	}
	return item, nil
}

// wishlistFxRate converts the item's amounts to the account's currency,
// keeping them as they are when the rate is unknown.
func wishlistFxRate(item *models.WishlistItem, acc *models.Account) float64 {
	if acc.Currency.String == item.Currency.String {
		return 1
	}
	if rate := item.Currency.FxRate(acc.Currency.String); rate > 0 {
		return rate
	}
	return 1
}

func mapWishlistItemToFD(item *models.WishlistItem, err *errs.ValidationError) *views.WishlistItemFD {
	p := item.Purchase()
	return &views.WishlistItemFD{
		Name:        form.FDInput{Value: item.Name, Error: err.Errors.MsgFor("Name")},
		PurchaseFD:  mapPurchaseToFD(&p, err.Errors),
		CoolingDays: form.FDInput{Value: strconv.Itoa(item.CoolingDays), Error: err.Errors.MsgFor("CoolingDays")},
		URL:         form.FDInput{Value: item.URL, Error: err.Errors.MsgFor("URL")},
		Note:        form.FDInput{Value: item.Note, Error: err.Errors.MsgFor("Note")},
	}
}

func mapFDToWishlistItem(r *http.Request, curs []models.Currency) (*models.WishlistItem, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	p, err := mapFDToPurchase(r, curs, &models.Currency{})
	if err != nil {
		return nil, err
	}

	days, _ := strconv.Atoi(r.FormValue("cooling_days"))

	return &models.WishlistItem{
		Name:        r.FormValue("name"),
		Price:       p.Price,
		Currency:    p.Currency,
		Plan:        p.Plan,
		URL:         r.FormValue("url"),
		Note:        r.FormValue("note"),
		CoolingDays: days,
	}, nil
}

// wishlistPlanTxs are the scheduled payments of a wishlist item's plan after
// first, the transaction buying it: its down payment, or its first monthly
// payment.
func wishlistPlanTxs(item *models.WishlistItem, first *models.Transaction) []models.Transaction {
	s := item.Schedule()
	if s == nil {
		return nil
	}

	day, from := first.ScheduledTo, 0
	if s.PaidOnDay() == 0 {
		day, from = day.AddDate(0, -s.Rows[0].Month, 0), 1
	}
	return planPaymentTxs(item.Name, s, first, day, from, wishlistFxRate(item, &first.Account))
}

// planPaymentTxs are the payments of a schedule bought on day, from its
// from-th one, like tmpl (account, kind, category, counterparty), the
// amounts converted with rate.
func planPaymentTxs(name string, s *models.InstallmentSchedule, tmpl *models.Transaction, day time.Time, from int, rate float64) []models.Transaction {
	rows := s.Rows[min(from, len(s.Rows)):]
	txs := make([]models.Transaction, len(rows))
	for i, row := range rows {
		txs[i] = models.Transaction{
			ID:           idgen.New(),
			Account:      tmpl.Account,
			Amount:       models.RoundCents(row.Payment * rate),
			Kind:         tmpl.Kind,
			Category:     tmpl.Category,
			Counterparty: tmpl.Counterparty,
			Description:  fmt.Sprintf("%s (%d/%d)", name, from+i+1, len(s.Rows)),
			ScheduledTo:  day.AddDate(0, row.Month, 0),
			CreatedAt:    time.Now(),
		}
	}
	return txs
}

func dfltWishlistItem(dfltCur *models.Currency) *models.WishlistItem {
	return &models.WishlistItem{Currency: *dfltCur, CoolingDays: models.WishlistCoolingDays[0]}
}
