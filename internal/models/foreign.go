package models

import (
	"math"
	"strings"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
)

// ForeignAmount is an amount in a currency other than its account's, by the
// currency's code (rates are keyed by code).
type ForeignAmount struct {
	Amount   float64
	Currency string
}

// View is the amount with its currency's code, e.g. "USD 10.5".
func (f *ForeignAmount) View() string {
	return formatters.FormatAmount(f.Amount, f.Currency)
}

// CanBeForeign reports whether a transaction of kind k can be paid in another
// currency than its account's: expenses, incomes and refunds.
func (k TxKind) CanBeForeign() bool {
	return k == ExpenseTxKind || k == IncomeTxKind || k == RefundTxKind
}

// TakesFxFee reports whether the account's foreign currency fee applies to a
// transaction of kind k: purchases only, money coming in is converted as is.
func (k TxKind) TakesFxFee() bool { return k == ExpenseTxKind }

// IsForeign reports whether it was paid in another currency than its
// account's.
func (t *Transaction) IsForeign() bool {
	return t.Original != nil && t.Original.Currency != "" &&
		!strings.EqualFold(t.Original.Currency, t.Account.Currency.String)
}

// OriginalRate is what one of its original currency cost in its account's,
// the fee aside; 0 when it isn't foreign.
func (t *Transaction) OriginalRate() float64 {
	if !t.IsForeign() || t.Original.Amount == 0 {
		return 0
	}
	return (math.Abs(t.Amount) - t.FxFee) / math.Abs(t.Original.Amount)
}

// PaidView tells what it was paid and the fee on top, e.g. "Paid USD 6.36
// + E£ 10.24 fee", or "Paid E£ 289.99 + E£ 8.69 fee" for a fee charged in
// the account's currency; "" when neither foreign nor with a fee.
func (t *Transaction) PaidView() string {
	var paid string
	switch {
	case t.IsForeign():
		paid = "Paid " + t.Original.View()
	case t.FxFee > 0:
		paid = "Paid " + t.Account.Currency.Format(math.Abs(t.Amount)-t.FxFee)
	default:
		return ""
	}
	if t.FxFee > 0 {
		paid += " + " + t.ViewFxFee() + " fee"
	}
	return paid
}

// ViewFxFee is its foreign currency fee in its account's currency.
func (t *Transaction) ViewFxFee() string {
	return t.Account.Currency.Format(t.FxFee)
}

// HasFxFee reports whether the account charges a fee on foreign purchases.
func (a *Account) HasFxFee() bool { return a.FxFeePercent > 0 || a.FxFeeMin > 0 }

// FxFee is the fee it charges on a foreign purchase of converted (in its
// currency): FxFeePercent of it, at least FxFeeMin; 0 without a fee.
func (a *Account) FxFee(converted float64) float64 {
	if !a.HasFxFee() || converted == 0 {
		return 0
	}
	fee := max(math.Abs(converted)*a.FxFeePercent/100, a.FxFeeMin)
	return a.Currency.Round(fee)
}

// ViewFxFee describes its foreign currency fee, e.g. "3% · min EGP 20", ""
// without one.
func (a *Account) ViewFxFee() string {
	var parts []string
	if a.FxFeePercent > 0 {
		parts = append(parts, formatters.FormatAmount(a.FxFeePercent, "")+"%")
	}
	if a.FxFeeMin > 0 {
		min := a.Currency.Format(a.FxFeeMin)
		if len(parts) > 0 {
			min = "min " + min
		}
		parts = append(parts, min)
	}
	return strings.Join(parts, " · ")
}

// ForeignCharge is what acc moves for amount of another currency at rate
// (acc's currency per one of it) for a transaction of kind: the converted
// amount plus, on a purchase, acc's fee, which is also returned alone.
func ForeignCharge(amount, rate float64, acc *Account, kind TxKind) (total, fee float64) {
	converted := acc.Currency.Round(amount * rate)
	if kind.TakesFxFee() {
		fee = acc.FxFee(converted)
	}
	return acc.Currency.Round(converted + fee), fee
}

// validateFxFee checks the fee is a percent of 0–100 and the minimum isn't
// negative.
func validateFxFee(pct, min float64) errs.ValidationErrors {
	ve := errs.ValidationErrors{}
	if pct < 0 {
		ve["FxFeePercent"] = errs.ValidationField{Tag: "gte", Param: "0"}
	} else if pct > 100 {
		ve["FxFeePercent"] = errs.ValidationField{Tag: "lte", Param: "100"}
	}
	if min < 0 {
		ve["FxFeeMin"] = errs.ValidationField{Tag: "gte", Param: "0"}
	}
	return ve
}
