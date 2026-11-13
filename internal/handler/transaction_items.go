package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// maxTxItems is how many lines a transaction takes.
const maxTxItems = 200

var nonSignedDigitRegexp = regexp.MustCompile(`[^0-9.\-]`)

// parseSignedFloat reads a number typed with separators or a unit, keeping
// its sign (a rounding line can be negative), 0 when blank or invalid.
func parseSignedFloat(s string) float64 {
	s = nonSignedDigitRegexp.ReplaceAllLiteralString(strings.TrimSpace(s), "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseRate(s string) *float64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	r := parseSignedFloat(s)
	return &r
}

// parseTxItems reads the line rows (parallel item_* fields, one value per
// row), dropping blank rows; never nil, so saving replaces the lines.
func parseTxItems(r *http.Request) []models.TxItem {
	_ = r.ParseForm()
	f := r.Form
	at := func(name string, i int) string {
		if vs := f[name]; i < len(vs) {
			return strings.TrimSpace(vs[i])
		}
		return ""
	}
	items := []models.TxItem{}
	for i := range min(len(f["item_name"]), maxTxItems) {
		kind, _ := strconv.Atoi(at("item_kind", i))
		it := models.TxItem{
			ID:        at("item_id", i),
			Kind:      models.TxItemKindOf(int64(kind)),
			Name:      at("item_name", i),
			Quantity:  parseSignedFloat(at("item_quantity", i)),
			UnitPrice: parseSignedFloat(at("item_unit_price", i)),
			Total:     parseSignedFloat(at("item_total", i)),
			Rate:      parseRate(at("item_rate", i)),
			Included:  at("item_included", i) != "",
		}
		if it.Name == "" && it.UnitPrice == 0 && it.Total == 0 && it.Rate == nil {
			continue
		}
		items = append(items, it)
	}
	return items
}

// itemsCurrencyFD is the lines' currency picker: the shown currencies (and
// the lines' own when hidden), the lines' picked (empty: the account's).
func (h Handler) itemsCurrencyFD(ctx context.Context, items []models.TxItem) views.ItemsCurrencyFD {
	fd := views.ItemsCurrencyFD{}
	cur := models.TxItemsCurrency(items, nil)
	if cur != nil {
		fd.Value = cur.ID
	}
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying the currencies for the lines", zap.Error(err))
	}
	fd.Currencies = curs
	if cur != nil && !slices.ContainsFunc(curs, func(c models.Currency) bool { return c.ID == cur.ID }) {
		fd.Currencies = append(fd.Currencies, *cur)
	}
	return fd
}

// parseItemsCurrency puts the lines in the form's items_currency_id (the
// account's when empty or unknown), the account (acc) resolved.
func (h Handler) parseItemsCurrency(r *http.Request, items []models.TxItem, acc *models.Currency) {
	cur := acc
	if id := strings.TrimSpace(r.FormValue("items_currency_id")); id != "" && (acc == nil || id != acc.ID) {
		c, err := h.repo.GetCurrency(r.Context(), id)
		if err != nil {
			logger.FromCtx(r.Context()).Warn("unknown lines' currency", zap.Error(err), zap.String("currency_id", id))
		} else {
			cur = c
		}
	}
	models.SetTxItemsCurrency(items, cur, acc)
}

// txItemsToFD maps lines to their inputs with their errors (keyed
// Items[i].Field).
func txItemsToFD(items []models.TxItem, err *errs.ValidationError) []views.TxItemFD {
	fds := views.TxItemsToFD(items)
	if err == nil {
		return fds
	}
	for i := range fds {
		for _, field := range []string{"Name", "Quantity", "Rate", "Kind"} {
			if msg := err.Errors.MsgFor(fmt.Sprintf("Items[%d].%s", i, field)); msg != "" {
				fds[i].Error = field + ": " + msg
				break
			}
		}
	}
	return fds
}

// normalizeTxItems fills what's missing of the transaction's lines in their
// currency (none of their own when it's the account's), copying them to the
// recurrent transaction it creates.
func normalizeTxItems(tx *models.Transaction) {
	acc := &tx.Account.Currency
	models.SetTxItemsCurrency(tx.Items, models.TxItemsCurrency(tx.Items, acc), acc)
	models.NormalizeTxItems(tx.Items, models.TxItemsCurrency(tx.Items, acc))
	tx.Items = models.SortTxItems(tx.Items)
	if tx.RecurrentTx != nil {
		tx.RecurrentTx.Items = models.CopyTxItems(tx.Items)
		if tx.RecurrentTx.Items == nil {
			tx.RecurrentTx.Items = []models.TxItem{}
		}
	}
}

// parseTxItemFD reads the details' add form: one line.
func parseTxItemFD(r *http.Request) (*models.TxItem, *views.TxItemFD) {
	kind, _ := strconv.Atoi(r.FormValue("kind"))
	it := &models.TxItem{
		Kind:      models.TxItemKindOf(int64(kind)),
		Name:      strings.TrimSpace(r.FormValue("name")),
		Quantity:  parseSignedFloat(r.FormValue("quantity")),
		UnitPrice: parseSignedFloat(r.FormValue("unit_price")),
		Total:     parseSignedFloat(r.FormValue("total")),
		Rate:      parseRate(r.FormValue("rate")),
		Included:  r.FormValue("included") != "",
	}
	fd := &views.TxItemFD{
		Kind: it.Kind, Name: it.Name, Included: it.Included,
		Quantity: r.FormValue("quantity"), UnitPrice: r.FormValue("unit_price"),
		Total: r.FormValue("total"), Rate: r.FormValue("rate"),
	}
	return it, fd
}

// validateTxItem checks one line, fd getting its error.
func (h Handler) validateTxItem(it *models.TxItem, fd *views.TxItemFD) bool {
	if it.Name == "" && !it.Kind.IsCharge() {
		fd.Error = "Name the item."
		return false
	}
	if it.Kind.IsCharge() && it.Total == 0 && it.Rate == nil {
		fd.Error = "Give its amount or rate."
		return false
	}
	if valerr := h.valid.Validate(it); valerr != nil {
		fd.Error = "Check the line's values."
		return false
	}
	return true
}

// editableTx is the transaction of the path whose lines can change.
func (h Handler) editableTx(r *http.Request) (*models.Transaction, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Transaction not found")
	}
	if err != nil {
		return nil, err
	}
	if tx.IsLocked() {
		return nil, errTxLocked
	}
	return tx, nil
}

// txItemsAnswer is the transaction details' lines, the row swapped out of
// band (its items chip).
func (h Handler) txItemsAnswer(w http.ResponseWriter, r *http.Request, id string, add *views.TxItemFD, rec *views.ReceiptFD) (templ.Component, error) {
	tx, err := h.repo.GetTransaction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		rec = &views.ReceiptFD{}
	}
	rec.Enabled = h.ocr != nil
	if add == nil {
		if err := h.renderTransactionsListOOB(w, r); err != nil {
			return nil, err
		}
	}
	return views.TxDetailsItems(tx, add, rec), nil
}

// AddTxItem adds a line to a transaction from its details.
func (h Handler) AddTxItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	it, fd := parseTxItemFD(r)
	if !h.validateTxItem(it, fd) {
		return h.txItemsAnswer(w, r, tx.ID, fd, nil)
	}
	it.Currency = models.TxItemsCurrency(tx.Items, nil)
	items := append(tx.Items, *it)
	models.NormalizeTxItems(items, models.TxItemsCurrency(items, &tx.Account.Currency))
	*it = items[len(items)-1]
	if err := h.repo.AddTxItem(r.Context(), tx.ID, it); err != nil {
		logger.FromCtx(r.Context()).Error("adding a line", zap.Error(err))
		return nil, err
	}
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}

// DeleteTxItem deletes a line of a transaction from its details.
func (h Handler) DeleteTxItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	err = h.repo.DeleteTxItem(r.Context(), tx.ID, r.PathValue("itemID"))
	if errors.Is(err, repository.ErrItemNotFound) {
		return nil, resperr.New(http.StatusNotFound, "Line not found")
	}
	if err != nil {
		return nil, err
	}
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}

// AddRecurrentTxItem adds a line to a recurrent transaction from its details,
// and to its scheduled occurrences not edited on their own.
func (h Handler) AddRecurrentTxItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	it, fd := parseTxItemFD(r)
	if !h.validateTxItem(it, fd) {
		return views.RecurrentTxDetailsItems(rt, fd), nil
	}
	it.Currency = models.TxItemsCurrency(rt.Items, nil)
	items := append(append([]models.TxItem{}, rt.Items...), *it)
	models.NormalizeTxItems(items, models.TxItemsCurrency(items, &rt.Account.Currency))
	*it = items[len(items)-1]
	if err := h.repo.AddRecurrentTxItem(r.Context(), rt, it); err != nil {
		logger.FromCtx(r.Context()).Error("adding a recurrent line", zap.Error(err))
		return nil, err
	}
	return views.RecurrentTxDetailsItems(rt, nil), nil
}

// DeleteRecurrentTxItem deletes a line of a recurrent transaction, and its
// copies on the scheduled occurrences not edited on their own.
func (h Handler) DeleteRecurrentTxItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rt, err := h.liveRecurrentTx(r)
	if err != nil {
		return nil, err
	}
	err = h.repo.DeleteRecurrentTxItem(r.Context(), rt, r.PathValue("itemID"))
	if errors.Is(err, repository.ErrItemNotFound) {
		return nil, resperr.New(http.StatusNotFound, "Line not found")
	}
	if err != nil {
		return nil, err
	}
	return views.RecurrentTxDetailsItems(rt, nil), nil
}

// Receipt serves a receipt's image, its url versioned (models.ReceiptVersion)
// so it's cached for good.
func (h Handler) Receipt(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rec, err := h.repo.GetReceipt(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Receipt not found")
	}
	if err != nil {
		return nil, err
	}
	etag := strconv.Quote(models.ReceiptVersion(rec.Hash, rec.ContentType))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil, nil
	}
	w.Header().Set("Content-Type", rec.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(rec.Data)))
	_, err = w.Write(rec.Data)
	return nil, err
}

// DeleteReceipt deletes a transaction's receipt image, its lines staying,
// answering the details' lines.
func (h Handler) DeleteReceipt(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rec, err := h.repo.GetReceipt(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Receipt not found")
	}
	if err != nil {
		return nil, err
	}
	if !rec.TransactionID.Valid {
		if err := h.repo.DeleteReceipt(r.Context(), rec.ID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	tx, err := h.repo.GetTransaction(r.Context(), rec.TransactionID.String)
	if err != nil {
		return nil, err
	}
	if tx.IsLocked() {
		return nil, errTxLocked
	}
	if err := h.repo.DeleteReceipt(r.Context(), rec.ID); err != nil {
		return nil, err
	}
	return h.txItemsAnswer(w, r, tx.ID, nil, nil)
}
