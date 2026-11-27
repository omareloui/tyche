package handler

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

var importMergeErrs = []error{
	models.ErrImportMergeNotImported, models.ErrImportMergeSame, models.ErrImportMergeAccount,
	models.ErrImportMergeKind, models.ErrImportMergeLocked,
}

// GetImportMergePicker offers the transactions an imported one can be merged
// into (in its row, or the details' panel with from=details).
func (h Handler) GetImportMergePicker(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.renderImportMergePicker(r, "")
}

// MergeImported merges the imported transaction into target_id: it takes
// the row's amount, fee and what it was paid in, keeping the rest (day,
// counterparty, category, counted in the balance or not); the imported one
// goes.
func (h Handler) MergeImported(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	imported, err := h.importMergeTx(r)
	if err != nil {
		return nil, err
	}
	target, err := h.repo.GetTransaction(ctx, r.FormValue("target_id"))
	if err != nil {
		return nil, errs.ErrInvalidID
	}
	if err := h.repo.MergeImportedInto(ctx, imported, target); err != nil {
		for _, e := range importMergeErrs {
			if errors.Is(err, e) {
				return h.renderImportMergePicker(r, e.Error())
			}
		}
		logger.FromCtx(ctx).Error("merging an imported transaction", zap.Error(err))
		return nil, err
	}

	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}
	if r.FormValue("from") != "details" {
		return nil, nil
	}
	// the details go on with the one merged into
	if cur, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && cur.Path != "" {
		q := cur.Query()
		if q.Has(views.TxDetailsParam) {
			q.Set(views.TxDetailsParam, target.ID)
			cur.RawQuery = q.Encode()
			w.Header().Set("HX-Replace-Url", cur.RequestURI())
		}
	}
	d, err := h.txDetails(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	return views.TxDetailsPanel(d, &getClaims(ctx).DefaultCurrency), nil
}

func (h Handler) renderImportMergePicker(r *http.Request, errMsg string) (templ.Component, error) {
	tx, err := h.importMergeTx(r)
	if err != nil {
		return nil, err
	}
	cands, err := h.repo.GetImportMergeCandidates(r.Context(), tx)
	if err != nil {
		return nil, err
	}
	return views.ImportMergePicker(tx, cands, errMsg, r.FormValue("from") == "details"), nil
}

// importMergeTx is the path's transaction when it's an imported one that
// can be merged.
func (h Handler) importMergeTx(r *http.Request) (*models.Transaction, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !tx.CanMergeImported() {
		return nil, resperr.New(http.StatusUnprocessableEntity, models.ErrImportMergeNotImported.Error())
	}
	return tx, nil
}
