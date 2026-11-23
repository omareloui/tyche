package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/resperr"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// parseTxTags reads a transaction form's tags (names, comma separated): nil
// when the form has no tags field, so its tags are left alone.
func parseTxTags(r *http.Request) []models.Tag {
	v := r.FormValue("tags") // parses the form, multipart ones too
	if !r.Form.Has("tags") {
		return nil
	}
	return tagsFromNames(models.ParseTagNames(v))
}

// tagsFromNames are the tags named so, resolved (or created) by name when
// saved; never nil.
func tagsFromNames(names []string) []models.Tag {
	out := make([]models.Tag, len(names))
	for i, n := range names {
		out[i] = models.Tag{Name: n}
	}
	return out
}

// tagsError is the first error of the tags field.
func tagsError(err *errs.ValidationError) string {
	if err == nil {
		return ""
	}
	if msg := err.Errors.MsgFor("Tags"); msg != "" {
		return msg
	}
	for k, f := range err.Errors {
		if strings.HasPrefix(k, "Tags[") {
			return f.String()
		}
	}
	return ""
}

// tagSuggestions are the live tags' names, offered by the tags inputs.
func (h Handler) tagSuggestions(ctx context.Context) []string {
	tags, err := h.repo.GetTags(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("fetching tags for suggestions", zap.Error(err))
		return nil
	}
	names := models.LiveTagNames(tags)
	if len(names) > maxTagSuggestions {
		names = names[:maxTagSuggestions]
	}
	return names
}

// maxTagSuggestions is how many tags the inputs offer (the trips first).
const maxTagSuggestions = 12

// txTagsFD is a transaction form's tags field.
func (h Handler) txTagsFD(ctx context.Context, tags []models.Tag, errMsg string) *views.TxTagsFD {
	return &views.TxTagsFD{
		FDInput:     form.FDInput{Value: strings.Join(models.TagNames(tags), ", "), Error: errMsg},
		Suggestions: h.tagSuggestions(ctx),
	}
}

// SetTxTags saves a transaction's tags from its details (the whole list in
// tags, plus a typed new one in add), answering its tags' editor and the
// list out of band.
func (h Handler) SetTxTags(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	if !views.TxTagsEditable(tx) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "This transaction can't be tagged.")
	}
	names := models.ParseTagNames(r.FormValue("tags") + "," + r.FormValue("add"))
	if msg := validateTagNames(names); msg != "" {
		return views.TxDetailsTags(tx, h.tagSuggestions(ctx), msg), nil
	}
	if err := h.repo.SetTransactionTags(ctx, tx.ID, tagsFromNames(names)); err != nil {
		logger.FromCtx(ctx).Error("saving transaction tags", zap.Error(err), zap.String("id", tx.ID))
		return nil, err
	}
	if tx, err = h.repo.GetTransaction(ctx, tx.ID); err != nil {
		return nil, err
	}
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	return views.TxDetailsTags(tx, h.tagSuggestions(ctx), ""), nil
}

// validateTagNames is what's wrong with the tags of a transaction, "" when
// nothing.
func validateTagNames(names []string) string {
	if len(names) > models.MaxTxTags {
		return fmt.Sprintf("A transaction takes up to %d tags.", models.MaxTxTags)
	}
	for _, n := range names {
		if utf8.RuneCountInString(n) > models.MaxTagNameLen {
			return fmt.Sprintf("A tag's name takes up to %d characters.", models.MaxTagNameLen)
		}
	}
	return ""
}
