package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// The *Note handlers save a note alone from the details' quick edit
// (PUT /<entity>/{id}/note with `note`), answering the editor (reopened with
// its error when refused) and what shows the note elsewhere out of band.

// parseNote is the posted note, trimmed, with its error when over max runes.
func parseNote(r *http.Request, max int) (string, string) {
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > max {
		return note, fmt.Sprintf("Keep it under %d characters.", max)
	}
	return note, ""
}

func notFoundOr(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return resperr.New(http.StatusNotFound, msg)
	}
	return err
}

// noteRefused is the editor reopened with the refused draft.
func noteRefused(p views.NoteEditorProps, draft, msg string) templ.Component {
	p.Draft, p.Error = draft, msg
	return views.NoteEditor(p)
}

func (h Handler) SetTransactionNote(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	if !views.TxNoteEditable(tx) {
		return nil, errTxLocked
	}
	p := views.NoteEditorProps{
		ID: views.NoteEditorID("tx", tx.ID), URL: fmt.Sprintf("/transactions/%s/note", tx.ID),
		Note: tx.Note, Editable: true,
	}
	note, msg := parseNote(r, models.MaxNoteLen)
	if msg != "" {
		return noteRefused(p, note, msg), nil
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetTransactionNote(r.Context(), tx.ID, note); err != nil {
		l.Error("setting transaction note", zap.Error(err), zap.String("id", tx.ID))
		return nil, err
	}
	l.Info("set transaction note", zap.String("id", tx.ID))
	// The row's note chip.
	if err := h.renderTransactionsListOOB(w, r); err != nil {
		return nil, err
	}
	p.Note = note
	return views.NoteEditor(p), nil
}

func (h Handler) SetRecurrentTxNote(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	p := views.NoteEditorProps{
		ID: views.NoteEditorID("rtx", rt.ID), URL: fmt.Sprintf("/recurrent-tx/%s/note", rt.ID),
		Note: rt.Note, Editable: true,
	}
	note, msg := parseNote(r, models.MaxNoteLen)
	if msg != "" {
		return noteRefused(p, note, msg), nil
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetRecurrentTxNote(r.Context(), rt.ID, note); err != nil {
		l.Error("setting recurrent tx note", zap.Error(err), zap.String("id", rt.ID))
		return nil, err
	}
	l.Info("set recurrent tx note", zap.String("id", rt.ID))
	p.Note = note
	return views.NoteEditor(p), nil
}

// SetAccountNote is behind LiveAccount.
func (h Handler) SetAccountNote(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	a, err := h.repo.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, errAccountNotFound
	}
	p := views.NoteEditorProps{
		ID: views.NoteEditorID("acc", a.ID), URL: fmt.Sprintf("/accounts/%s/note", a.ID),
		Note: a.Note, Editable: true,
	}
	note, msg := parseNote(r, models.MaxNoteLen)
	if msg != "" {
		return noteRefused(p, note, msg), nil
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetAccountNote(r.Context(), a.ID, note); err != nil {
		l.Error("setting account note", zap.Error(err), zap.String("id", a.ID))
		return nil, err
	}
	l.Info("set account note", zap.String("id", a.ID))
	a.Note = note
	if err := renderToBody(w, r, views.AccountReplaceOOB(a, &getClaims(r.Context()).DefaultCurrency)); err != nil {
		return nil, err
	}
	p.Note = note
	return views.NoteEditor(p), nil
}

func (h Handler) SetWishlistItemNote(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	item, err := h.repo.GetWishlistItem(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, notFoundOr(err, "Wish not found")
	}
	p := views.NoteEditorProps{
		ID: views.NoteEditorID("wish", item.ID), URL: fmt.Sprintf("/wishlist/%s/note", item.ID),
		Note: item.Note, Editable: true,
	}
	note, msg := parseNote(r, models.MaxNoteLen)
	if msg != "" {
		return noteRefused(p, note, msg), nil
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetWishlistItemNote(r.Context(), item.ID, note); err != nil {
		l.Error("setting wish note", zap.Error(err), zap.String("id", item.ID))
		return nil, err
	}
	l.Info("set wish note", zap.String("id", item.ID))
	// Its card's note line.
	list, err := h.wishlist(r, true)
	if err != nil {
		return nil, err
	}
	if err := renderToBody(w, r, list); err != nil {
		return nil, err
	}
	p.Note = note
	return views.NoteEditor(p), nil
}

func (h Handler) SetCounterpartyNote(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	cp, err := h.repo.GetCounterparty(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, notFoundOr(err, "Counterparty not found")
	}
	p := views.NoteEditorProps{
		ID: views.NoteEditorID("cp", cp.ID), URL: fmt.Sprintf("/counterparties/%s/note", cp.ID),
		Note: cp.Note, Editable: true,
	}
	note, msg := parseNote(r, models.MaxCounterpartyNoteLen)
	if msg != "" {
		return noteRefused(p, note, msg), nil
	}
	l := logger.FromCtx(r.Context())
	if err := h.repo.SetCounterpartyNote(r.Context(), cp.ID, note); err != nil {
		l.Error("setting counterparty note", zap.Error(err), zap.String("id", cp.ID))
		return nil, err
	}
	l.Info("set counterparty note", zap.String("id", cp.ID))
	cp.Note = note
	if err := renderToBody(w, r, views.CounterpartyReplaceOOB(cp)); err != nil {
		return nil, err
	}
	p.Note = note
	return views.NoteEditor(p), nil
}
