package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// TransactionDetails is the drawer showing a transaction in full, with
// what's related to it.
func (h Handler) TransactionDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	d, err := h.txDetails(r.Context(), id)
	if err != nil {
		return nil, err
	}

	def := &getClaims(r.Context()).DefaultCurrency
	// With the drawer open, only its content is swapped.
	if r.Header.Get("HX-Target") == txDetailsPanel {
		return views.TxDetailsPanel(d, def), nil
	}
	return views.TxDetails(d, def), nil
}

// txDetailsPanel is the id of the open drawer's content.
const txDetailsPanel = "txd-panel"

func (h Handler) txDetails(ctx context.Context, id string) (*models.TxDetails, error) {
	d, err := h.repo.GetTransactionDetails(ctx, id, &getClaims(ctx).DefaultCurrency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "transaction not found")
	}
	if err != nil {
		logger.FromCtx(ctx).Error("fetching transaction details", zap.Error(err), zap.String("id", id))
		return nil, err
	}

	if d.SMS != nil && d.SMS.TemplateID != "" {
		d.SMSTemplate = h.smsTemplateName(ctx, d.SMS.TemplateID)
	}
	d.ReceiptScan = h.ocr != nil
	if d.Tx.Kind == models.ExpenseTxKind && len(d.Tx.Items) > 0 {
		s, err := h.repo.TxSplitOf(ctx, d.Tx)
		if err != nil {
			return nil, err
		}
		if len(s.People) > 0 {
			d.Split = s
		}
	}
	return d, nil
}

// smsTemplateName is the name of the template of id, the user's own or a
// shared one, empty if it's gone.
func (h Handler) smsTemplateName(ctx context.Context, id string) string {
	if t, err := h.repo.GetSMSTemplate(ctx, id); err == nil {
		return t.Name
	}
	if t, err := h.smsTemplates.Get(id); err == nil {
		return t.Name
	}
	return ""
}
