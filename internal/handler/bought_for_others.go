package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

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

// GetTxForOthersForm answers the details' "I bought it for someone" with its
// form.
func (h Handler) GetTxForOthersForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderTxForOthersForm(r, "")
}

func (h Handler) renderTxForOthersForm(r *http.Request, errMsg string) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !views.TxForOthersEditable(tx) {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrForOthersKind.Error())
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, err
	}
	return views.TxForOthersForm(tx, cps, errMsg), nil
}

// SetTxForOthers marks the expense in the path bought for someone: for_person
// (none when empty; keep_person keeps who it's for), payback scheduling the
// income paying back what isn't yet. Answers the details again.
func (h Handler) SetTxForOthers(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	refused := func(msg string) (templ.Component, error) {
		w.Header().Set("HX-Retarget", "#"+views.TxForOthersID(id))
		w.Header().Set("HX-Reswap", "outerHTML")
		return h.renderTxForOthersForm(r, msg)
	}

	var person *models.Counterparty
	if r.FormValue("keep_person") != "" {
		tx, err := h.repo.GetTransaction(r.Context(), id)
		if err != nil {
			return nil, err
		}
		person = tx.ForPerson
	} else if pid := strings.TrimSpace(r.FormValue("for_person")); pid != "" {
		cp, err := h.repo.GetCounterparty(r.Context(), pid)
		if err != nil {
			return refused("Pick someone from the list")
		}
		person = cp
	}

	payback := r.FormValue("payback") != ""
	if err := h.repo.SetForOthers(r.Context(), id, true, person, payback, h.userToday(r.Context())); err != nil {
		for _, e := range forOthersErrs {
			if errors.Is(err, e) {
				return refused(e.Error())
			}
		}
		l.Error("setting bought for others", zap.Error(err))
		return nil, err
	}
	l.Info("set bought for others", zap.Bool("payback", payback))
	return h.answerBoughtForLink(w, r)
}

// UnsetTxForOthers makes the expense in the path the user's again.
func (h Handler) UnsetTxForOthers(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.SetForOthers(r.Context(), id, false, nil, false, time.Time{}); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("unset bought for others")
	return h.answerBoughtForLink(w, r)
}
