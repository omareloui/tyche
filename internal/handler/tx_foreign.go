package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// The transaction form's "Paid in" fields: the currency it was paid in (a
// code, empty = the account's), the amount in it, and the account's fee the
// form worked out last (kept as is when the amount is typed by hand).
const (
	originalCurrencyField = "original_currency"
	originalAmountField   = "original_amount"
	fxFeeField            = "fx_fee"
	// fxFeeUnitField is fxFeeUnitPct when the fee is typed as a percent of
	// what was paid, else it's an amount.
	fxFeeUnitField = "fx_fee_unit"
	fxFeeUnitPct   = "pct"
)

// foreignCurrency is the form's paid in currency, "" when it's acc's.
func foreignCurrency(r *http.Request, acc *models.Account) string {
	cur := strings.ToUpper(strings.TrimSpace(r.FormValue(originalCurrencyField)))
	if acc == nil || strings.EqualFold(cur, acc.Currency.String) {
		return ""
	}
	return cur
}

// txFormFee is the fee the form shows apart from the amount: a purchase's.
func txFormFee(t *models.Transaction) float64 {
	if t.Kind.TakesFxFee() && t.FxFee > 0 && t.FxFee < t.Amount {
		return t.FxFee
	}
	return 0
}

// applyForeign sets what a transaction of the form was paid in and its fee,
// once its account is resolved: nothing when the form has no "Paid in" field
// (other forms), cleared when it's the account's currency or a kind that
// can't be foreign. The form's amount is what was paid, the fee (a
// purchase's) on top of it: the transaction's amount is both. Without an
// amount typed a foreign one is worked out at the day's rate, the fee being
// the account's unless typed.
func (h Handler) applyForeign(r *http.Request, tx *models.Transaction) {
	if !r.Form.Has(originalCurrencyField) {
		return
	}
	tx.Original, tx.FxFee = nil, 0
	fee, _ := parseFloatIfExists(r.FormValue(fxFeeField))
	feeTyped := strings.TrimSpace(r.FormValue(fxFeeField)) != ""
	feePct := r.FormValue(fxFeeUnitField) == fxFeeUnitPct
	defer func() {
		if feePct && feeTyped {
			// a percent of what was paid
			fee = tx.Amount * min(fee, 100) / 100
		}
		if tx.Kind.TakesFxFee() && fee > 0 && tx.Amount > 0 {
			tx.FxFee = tx.Account.Currency.Round(fee)
			tx.Amount = tx.Account.Currency.Round(tx.Amount + tx.FxFee)
		}
	}()

	cur := foreignCurrency(r, &tx.Account)
	orig, _ := parseFloatIfExists(r.FormValue(originalAmountField))
	if cur == "" || orig <= 0 || !tx.Kind.CanBeForeign() || tx.Account.Currency.IsMetal() {
		return
	}
	tx.Original = &models.ForeignAmount{Amount: orig, Currency: cur}
	if tx.Amount == 0 {
		if rate := h.transferRate(r, cur, tx.Account.Currency.String); rate > 0 {
			total, accFee := models.ForeignCharge(orig, rate, &tx.Account, tx.Kind)
			tx.Amount = total - accFee
			if !feeTyped {
				fee = accFee
			}
		}
	}
}

// foreignCurrencies are the currencies a transaction can be paid in, the
// picked one (cur) kept when it isn't shown.
func (h Handler) foreignCurrencies(ctx context.Context, cur string) []models.Currency {
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying the currencies to pay in", zap.Error(err))
	}
	curs = slices.DeleteFunc(curs, func(c models.Currency) bool { return c.IsMetal() })
	if cur != "" && !slices.ContainsFunc(curs, func(c models.Currency) bool { return strings.EqualFold(c.String, cur) }) {
		curs = append(curs, models.Currency{String: cur, Symbol: cur})
	}
	return curs
}

// txForeignFD is the "Paid in" fields of a transaction's form, nil when it
// can't be foreign (its kind, a metal account).
func (h Handler) txForeignFD(ctx context.Context, t *models.Transaction) *views.TxForeignFD {
	if !t.Kind.CanBeForeign() || t.Account.Currency.IsMetal() {
		return nil
	}
	fd := &views.TxForeignFD{AccountCurrency: t.Account.Currency.String}
	if t.IsForeign() {
		fd.Currency.Value = strings.ToUpper(t.Original.Currency)
		fd.Amount.Value = formatFloatIfNonZero(t.Original.Amount)
		fd.Hint = foreignHint(t.OriginalRate(), 0, fd.Currency.Value, &t.Account)
	}
	fd.Fee = formatFloatIfNonZero(txFormFee(t))
	fd.TakesFee = t.Kind.TakesFxFee()
	fd.Currencies = h.foreignCurrencies(ctx, fd.Currency.Value)
	return fd
}

// parseTxForeignFD is the "Paid in" fields as the form sent them, the
// amount worked out again (into the form's amount) when one of them, the
// account or the kind changed; nil when it can't be foreign. Its currencies
// are filled by fillForeignCurrencies.
func (h Handler) parseTxForeignFD(r *http.Request, fd *views.TransactionAmountsAndKindFD, acc *models.Account, kind models.TxKind, triggered func(...string) bool) *views.TxForeignFD {
	if acc == nil || !kind.CanBeForeign() || acc.Currency.IsMetal() {
		return nil
	}
	cur := foreignCurrency(r, acc)
	ffd := &views.TxForeignFD{
		AccountCurrency: acc.Currency.String,
		Currency:        form.FDInput{Value: cur},
		Amount:          form.FDInput{Value: r.FormValue(originalAmountField)},
		Fee:             r.FormValue(fxFeeField),
		FeePct:          r.FormValue(fxFeeUnitField) == fxFeeUnitPct,
		TakesFee:        kind.TakesFxFee(),
	}
	if !kind.TakesFxFee() {
		ffd.Fee = ""
	}
	if cur == "" {
		return ffd
	}
	orig, _ := parseFloatIfExists(ffd.Amount.Value)
	rate := h.transferRate(r, cur, acc.Currency.String)
	if rate <= 0 {
		ffd.Hint = fmt.Sprintf("No rate for %s to %s, type the amount charged", cur, acc.Currency.String)
		return ffd
	}
	// what was paid converted into the amount, the account's fee apart
	if orig > 0 && triggered(originalAmountField, originalCurrencyField, "account_id", "kind") {
		total, fee := models.ForeignCharge(orig, rate, acc, kind)
		fd.AccountIFD.Amount.Value = strconv.FormatFloat(total-fee, 'f', 2, 64)
		if kind.TakesFxFee() {
			ffd.Fee, ffd.FeePct = formatFloatIfNonZero(fee), false
		}
	}
	ffd.Hint = foreignHint(rate, 0, cur, acc)
	return ffd
}

// foreignHint tells the rate a foreign amount converts at and the fee added,
// e.g. "1 USD = 50.12 EGP · + EGP 15 fee".
func foreignHint(rate, fee float64, cur string, acc *models.Account) string {
	if rate <= 0 {
		return ""
	}
	hint := fmt.Sprintf("1 %s = %s", cur, acc.Currency.Format(rate))
	if fee > 0 {
		hint += " · + " + acc.Currency.Format(fee) + " fee"
	}
	return hint
}

// fillForeignCurrencies lists the currencies of a parsed form's "Paid in".
func (h Handler) fillForeignCurrencies(ctx context.Context, fd *views.TransactionAmountsAndKindFD) {
	if fd.Foreign != nil {
		fd.Foreign.Currencies = h.foreignCurrencies(ctx, fd.Foreign.Currency.Value)
	}
}
