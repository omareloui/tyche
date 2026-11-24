package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

var forOthersErrs = []error{
	models.ErrForOthersKind,
	models.ErrForOthersBoughtBy,
	models.ErrForOthersSplit,
}

// forOthersFD is the create form's "I bought it for someone" as sent.
func forOthersFD(r *http.Request) views.ForOthersFD {
	return views.ForOthersFD{
		On:       form.FDInput{Value: r.FormValue("for_others_on")},
		PersonID: form.FDInput{Value: r.FormValue("for_person")},
		Payback:  form.FDInput{Value: r.FormValue("for_others_payback")},
	}
}

// forOthers is what the create form says of an expense bought for someone.
type forOthers struct {
	Person  *models.Counterparty
	Payback bool
}

// forOthersOf reads the create form's "I bought it for someone": nil when
// it's off (or the tx isn't an expense), else who for and whether they'll pay
// it back. The returned form data, when not nil, is the field with its error
// to show again.
func forOthersOf(r *http.Request, tx *models.Transaction, cps []models.Counterparty, boughtBy, wish bool) (*forOthers, *views.ForOthersFD) {
	fd := forOthersFD(r)
	if fd.On.Value == "" || tx.Kind != models.ExpenseTxKind {
		return nil, nil
	}
	switch {
	case tx.RecurrentTx != nil:
		fd.On.Error = "Only a one time expense can be bought for someone, not a repeating one"
	case boughtBy:
		fd.On.Error = models.ErrForOthersBoughtBy.Error()
	case wish:
		fd.On.Error = "A wishlist item you buy is yours, not bought for someone"
	case !tx.CanBeForOthers():
		fd.On.Error = models.ErrForOthersKind.Error()
	}
	var person *models.Counterparty
	if id := strings.TrimSpace(fd.PersonID.Value); id != "" {
		person = slicesutils.Find(cps, func(c models.Counterparty) bool { return c.ID == id })
		if person == nil {
			fd.PersonID.Error = "Pick someone from the list"
		}
	}
	if fd.On.Error != "" || fd.PersonID.Error != "" {
		return nil, &fd
	}
	return &forOthers{Person: person, Payback: fd.Payback.Value != ""}, nil
}

// GetTxForOthersPicker answers the details' "Bought for" quick pick.
func (h Handler) GetTxForOthersPicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.forOthersTx(r)
	if err != nil {
		return nil, err
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, err
	}
	return views.TxForOthersPicker(tx, cps), nil
}

// forOthersTx is the {id} expense when the details can change who it was
// bought for.
func (h Handler) forOthersTx(r *http.Request) (*models.Transaction, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !views.TxForOthersEditable(tx) && !(tx.ForOthers && tx.CanBeForOthers()) {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrForOthersKind.Error())
	}
	return tx, nil
}

// SetTxForOthers sets who the expense in the path was bought for: for_person
// "me" makes it the user's again, "someone" for someone unnamed, else a
// counterparty's id; payback schedules the income paying back what isn't
// yet. Answers the details and the list out of band.
func (h Handler) SetTxForOthers(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	tx, err := h.forOthersTx(r)
	if err != nil {
		return nil, err
	}
	pickID := views.QuickPickID("tx", tx.ID, views.QuickPickForOthers)

	on := true
	var person *models.Counterparty
	switch v := strings.TrimSpace(r.FormValue("for_person")); v {
	case views.ForOthersMe, "":
		on = false
	case views.ForOthersSomeone:
	default:
		if person, err = h.repo.GetCounterparty(ctx, v); err != nil {
			return nil, pickRefused(w, pickID, "Pick someone from the list.")
		}
	}

	payback := r.FormValue("payback") != ""
	if err := h.repo.SetForOthers(ctx, tx.ID, on, person, payback, h.userToday(ctx)); err != nil {
		for _, e := range forOthersErrs {
			if errors.Is(err, e) {
				return nil, pickRefused(w, pickID, e.Error())
			}
		}
		l.Error("setting bought for others", zap.Error(err))
		return nil, err
	}
	l.Info("set bought for others", zap.Bool("on", on), zap.Bool("payback", payback))

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	d, err := h.txDetails(ctx, tx.ID)
	if err != nil {
		return nil, err
	}
	return views.TxDetailsPanel(d, &getClaims(ctx).DefaultCurrency), nil
}
