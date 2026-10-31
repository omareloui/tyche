package handler

import (
	"errors"
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

	item, missingInstallments, err := mapFDToWishlistItem(r, curs)
	if err != nil {
		l.Error("parsing form to wishlist item", zap.Error(err))
		return nil, err
	}
	item.ID = idgen.New()
	item.CreatedAt = time.Now()

	claims := getClaims(r.Context())

	if err := h.validateWishlistItem(item, missingInstallments); err != nil {
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

	item, missingInstallments, err := mapFDToWishlistItem(r, curs)
	if err != nil {
		return nil, err
	}
	item.ID = id

	if err := h.validateWishlistItem(item, missingInstallments); err != nil {
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

	amount := item.Price
	if item.IsInstallments() {
		amount = item.InstallmentAmount
		now := time.Now()
		tx.RecurrentTx = &models.RecurrentTx{
			Unit:    models.RecurrentUnitMonth,
			Every:   1,
			StopsAt: now.AddDate(0, item.Installments-1, 0),
		}
	}
	if tx.Account.Currency.String != item.Currency.String {
		amount *= item.Currency.FxRate(tx.Account.Currency.String)
	}
	tx.Amount = amount
	tx.Description = item.Name
	applyCounterpartyDefaultCategory(tx, cps)

	fd := h.mapTransactionToFD(tx, new(errs.ValidationError))
	fd.WishlistID = item.ID

	claims := getClaims(r.Context())
	return views.WishlistBuySheet(views.CreateTransactionForm(fd, accs, knds, cats, cps, gls, &claims.DefaultCurrency)), nil
}

func mapWishlistItemToFD(item *models.WishlistItem, err *errs.ValidationError) *views.WishlistItemFD {
	p := &models.Purchase{
		Price:             item.Price,
		Currency:          item.Currency,
		Installments:      item.Installments,
		InstallmentAmount: item.InstallmentAmount,
	}
	pfd := mapPurchaseToFD(p)
	pfd.AmountFD.Amount.Error = err.Errors.MsgFor("Price")
	pfd.AmountFD.CurrencyID.Error = err.Errors.MsgFor("CurrencyID")
	pfd.Installments.Error = err.Errors.MsgFor("Installments")
	pfd.InstallmentAmount.Error = err.Errors.MsgFor("InstallmentAmount")
	if pfd.Installments.Error != "" || pfd.InstallmentAmount.Error != "" {
		pfd.Mode.Value = views.PayInstallments
	}

	return &views.WishlistItemFD{
		Name:        form.FDInput{Value: item.Name, Error: err.Errors.MsgFor("Name")},
		PurchaseFD:  pfd,
		CoolingDays: form.FDInput{Value: strconv.Itoa(item.CoolingDays), Error: err.Errors.MsgFor("CoolingDays")},
		URL:         form.FDInput{Value: item.URL, Error: err.Errors.MsgFor("URL")},
		Note:        form.FDInput{Value: item.Note, Error: err.Errors.MsgFor("Note")},
	}
}

// mapFDToWishlistItem reads the item, and whether it's paid in installments
// with neither their count nor amount given.
func mapFDToWishlistItem(r *http.Request, curs []models.Currency) (*models.WishlistItem, bool, error) {
	if err := r.ParseForm(); err != nil {
		return nil, false, err
	}

	p, err := mapFDToPurchase(r, curs, &models.Currency{})
	missingInstallments := errors.Is(err, errNoInstallments)
	if err != nil && !missingInstallments {
		return nil, false, err
	}

	days, _ := strconv.Atoi(r.FormValue("cooling_days"))

	return &models.WishlistItem{
		Name:              r.FormValue("name"),
		Price:             p.Price,
		Currency:          p.Currency,
		Installments:      p.Installments,
		InstallmentAmount: p.InstallmentAmount,
		URL:               r.FormValue("url"),
		Note:              r.FormValue("note"),
		CoolingDays:       days,
	}, missingInstallments, nil
}

// validateWishlistItem sanitizes and validates the item, also asking for the
// installments when paying in them without any.
func (h Handler) validateWishlistItem(item *models.WishlistItem, missingInstallments bool) error {
	err := item.SanitizeAndValidate(h.sani, h.valid)
	if !missingInstallments {
		return err
	}

	var valerr errs.ValidationError
	if err != nil && !errors.As(err, &valerr) {
		return err
	}
	if valerr.Errors == nil {
		valerr.Errors = errs.ValidationErrors{}
	}
	valerr.Errors["Installments"] = errs.ValidationField{Tag: "months_or_amount"}
	return valerr
}

func dfltWishlistItem(dfltCur *models.Currency) *models.WishlistItem {
	return &models.WishlistItem{Currency: *dfltCur, CoolingDays: models.WishlistCoolingDays[0]}
}
