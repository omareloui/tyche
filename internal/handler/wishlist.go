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
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// wishlistCreatedEvent closes the create form.
const wishlistCreatedEvent = "wishlist-created"

// Wishlist renders the whole wishlist, which every change on it re-renders.
func (h Handler) Wishlist(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.wishlist(r, false)
}

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

	list, err := h.wishlist(r, true)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, list); err != nil {
		return nil, err
	}

	w.Header().Set("HX-Trigger", wishlistCreatedEvent)
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
	return views.EditWishlistItem(id, mapWishlistItemToFD(item, &errs.ValidationError{}), curs, &claims.DefaultCurrency), nil
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
			// The form targets the whole wishlist, the errors go in its place.
			w.Header().Set("HX-Retarget", "#"+views.WishlistItemID(id))
			claims := getClaims(r.Context())
			return views.EditWishlistItem(id, mapWishlistItemToFD(item, &valerr), curs, &claims.DefaultCurrency), nil
		}
		return nil, err
	}

	if err := h.repo.UpdateWishlistItem(r.Context(), item); err != nil {
		l.Error("updating wishlist item", zap.Error(err))
		return nil, err
	}
	l.Info("updated wishlist item", zap.String("id", id))

	return h.wishlist(r, false)
}

func (h Handler) AbandonWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setWishlistItemStatus(r, models.WishlistAbandoned)
}

// RestoreWishlistItem puts a decided item back to waiting.
func (h Handler) RestoreWishlistItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.setWishlistItemStatus(r, models.WishlistWaiting)
}

func (h Handler) setWishlistItemStatus(r *http.Request, status models.WishlistStatus) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.SetWishlistItemStatus(r.Context(), id, status, ""); err != nil {
		return nil, err
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

	accs, knds, cats, cps, gls, err := h.getTxDeps(r.Context())
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

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))
	fd.WishlistID = item.ID

	claims := getClaims(r.Context())
	return views.WishlistBuySheet(views.CreateTransactionForm(fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency)), nil
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

	rows := s.Rows
	day := first.ScheduledTo
	if s.PaidOnDay() == 0 {
		day = day.AddDate(0, -rows[0].Month, 0)
		rows = rows[1:]
	}

	rate := wishlistFxRate(item, &first.Account)

	txs := make([]models.Transaction, len(rows))
	for i, row := range rows {
		txs[i] = models.Transaction{
			ID:           idgen.New(),
			Account:      first.Account,
			Amount:       models.RoundCents(row.Payment * rate),
			Kind:         first.Kind,
			Category:     first.Category,
			Counterparty: first.Counterparty,
			Description:  fmt.Sprintf("%s (%d/%d)", item.Name, len(s.Rows)-len(rows)+i+1, len(s.Rows)),
			ScheduledTo:  day.AddDate(0, row.Month, 0),
			CreatedAt:    time.Now(),
		}
	}
	return txs
}

func dfltWishlistItem(dfltCur *models.Currency) *models.WishlistItem {
	return &models.WishlistItem{Currency: *dfltCur, CoolingDays: models.WishlistCoolingDays[0]}
}
