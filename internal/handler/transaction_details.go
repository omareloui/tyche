package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
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

	ctx := r.Context()
	def := &getClaims(ctx).DefaultCurrency

	d, err := h.repo.GetTransactionDetails(ctx, id, def)
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

	return views.TxDetails(d, def), nil
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
