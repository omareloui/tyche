package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Return + warranty windows: until when a purchase (an expense) can be
// returned (the transaction's, a store's policy covers the receipt) and is
// under warranty (each item line's own, else its transaction's for the
// whole purchase). Every date is a user day at midnight UTC, like the days
// picked in forms, compared with Handler.userToday.

const (
	// ReturnSoonDays is how close a return window's end gets reminded of
	// and shown amber.
	ReturnSoonDays = 3
	// WarrantySoonDays is how close a warranty's end gets reminded of.
	WarrantySoonDays = 30
)

// ReturnPresets are the return windows offered, in days from the purchase.
var ReturnPresets = []int{14, 30}

// WarrantyPresets are the warranties offered, in months from the purchase.
var WarrantyPresets = []int{12, 24}

// TxWindowsAllowed tells a transaction may carry return / warranty windows
// once an expense: not locked (goals', deleted accounts'), a recurrent
// occurrence, a transfer's leg or a loan's payment.
func TxWindowsAllowed(t *Transaction) bool {
	return !t.IsLocked() && t.RecurrentTxID == "" && t.RecurrentTx == nil &&
		!t.IsGoalPurchase() && !t.IsTransferLeg() && !t.IsLoanPayment()
}

// TxWindowsEditable tells a transaction can carry return / warranty
// windows: an expense TxWindowsAllowed.
func TxWindowsEditable(t *Transaction) bool {
	return t.Kind == ExpenseTxKind && TxWindowsAllowed(t)
}

// HasWindows tells it has a return or warranty date set (on it or a line).
func (t *Transaction) HasWindows() bool {
	return t.ReturnUntil != nil || t.LatestWarranty() != nil
}

// ReturnOpen tells it can still be returned on today: a return window not
// over and not returned yet.
func (t *Transaction) ReturnOpen(today time.Time) bool {
	return t.ReturnUntil != nil && t.ReturnedAt == nil && !t.ReturnUntil.Before(today)
}

// ReturnSoon tells its open return window ends within ReturnSoonDays.
func (t *Transaction) ReturnSoon(today time.Time) bool {
	return t.ReturnOpen(today) && DaysUntil(*t.ReturnUntil, today) <= ReturnSoonDays
}

// LatestWarranty is the latest warranty of the purchase: its own or a
// line's (the loaded lines', else LinesWarranty from the list), nil when
// none.
func (t *Transaction) LatestWarranty() *time.Time {
	latest := t.WarrantyUntil
	later := func(d *time.Time) {
		if d != nil && (latest == nil || d.After(*latest)) {
			latest = d
		}
	}
	later(t.LinesWarranty)
	for i := range t.Items {
		later(t.Items[i].WarrantyUntil)
	}
	return latest
}

// EffectiveWarranty is an item line's warranty: its own, else its
// transaction's (the whole purchase's); never a charge's.
func (it *TxItem) EffectiveWarranty(tx *Transaction) (d *time.Time, own bool) {
	if it.Kind.IsCharge() {
		return nil, false
	}
	if it.WarrantyUntil != nil {
		return it.WarrantyUntil, true
	}
	if tx != nil {
		return tx.WarrantyUntil, false
	}
	return nil, false
}

// WarrantyActive tells a warranty ending on d still covers today.
func WarrantyActive(d *time.Time, today time.Time) bool {
	return d != nil && !d.Before(today)
}

// WarrantySoon tells a warranty ending on d is active and ends within
// WarrantySoonDays.
func WarrantySoon(d *time.Time, today time.Time) bool {
	return WarrantyActive(d, today) && DaysUntil(*d, today) <= WarrantySoonDays
}

// DaysUntil is how many days from today to d (both days), negative when
// past.
func DaysUntil(d, today time.Time) int {
	return int(DayOf(d).Sub(DayOf(today)).Hours() / 24)
}

// DayOf is t's day at midnight UTC, as the windows are.
func DayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ParseWindowValue reads a window picked from the purchase's day: "14d"
// (days), "12m" (months), a day (YYYY-MM-DD) or "" (none, nil).
func ParseWindowValue(v string, purchase time.Time) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if n, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days < 1 || days > 3650 {
			return nil, fmt.Errorf("invalid days %q", v)
		}
		d := DayOf(purchase).AddDate(0, 0, days)
		return &d, nil
	}
	if n, ok := strings.CutSuffix(v, "m"); ok {
		months, err := strconv.Atoi(n)
		if err != nil || months < 1 || months > 240 {
			return nil, fmt.Errorf("invalid months %q", v)
		}
		d := AddMonthsClamped(DayOf(purchase), months)
		return &d, nil
	}
	d, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q", v)
	}
	return &d, nil
}

// ReturnUntilFor is the return window a purchase on day gets from its
// merchant's usual one, nil when it has none.
func ReturnUntilFor(day time.Time, cp *Counterparty) *time.Time {
	if cp == nil || !cp.IsMerchant || cp.ReturnDays <= 0 {
		return nil
	}
	d := DayOf(day).AddDate(0, 0, cp.ReturnDays)
	return &d
}

// WindowsFromReceipt is the windows a receipt prints (days to return,
// months of warranty, 0 = not printed) from the purchase's day.
func WindowsFromReceipt(day time.Time, returnDays, warrantyMonths int) (ret, war *time.Time) {
	if returnDays > 0 && returnDays <= 3650 {
		d := DayOf(day).AddDate(0, 0, returnDays)
		ret = &d
	}
	if warrantyMonths > 0 && warrantyMonths <= 240 {
		d := AddMonthsClamped(DayOf(day), warrantyMonths)
		war = &d
	}
	return ret, war
}
