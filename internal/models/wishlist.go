package models

import (
	"math"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type WishlistStatus uint8

const (
	WishlistWaiting WishlistStatus = iota
	WishlistBought
	WishlistAbandoned
)

// WishlistCoolingDays are the cooling-off periods to pick from.
var WishlistCoolingDays = []int{7, 30}

// WishlistItem is something to buy only once its cooling-off period is over,
// unless it gets abandoned in the meantime.
type WishlistItem struct {
	ID       string
	Name     string
	Price    float64
	Currency Currency
	// Plan is how it's paid in installments, nil to pay cash.
	Plan          *InstallmentPlan
	URL           string
	Note          string
	CoolingDays   int
	Status        WishlistStatus
	DecidedAt     time.Time
	TransactionID string
	CreatedAt     time.Time
}

type createWishlistItem struct {
	ID          string  `validate:"required"`
	Name        string  `validate:"required" conform:"trim"`
	Price       float64 `validate:"required,gt=0"`
	CurrencyID  string  `validate:"required"`
	URL         string  `validate:"omitempty,http_url" conform:"trim"`
	Note        string  `conform:"trim"`
	CoolingDays int     `validate:"oneof=7 30"`
}

func (w *WishlistItem) toValidate() *createWishlistItem {
	return &createWishlistItem{
		ID:          w.ID,
		Name:        w.Name,
		Price:       w.Price,
		CurrencyID:  w.Currency.ID,
		URL:         w.URL,
		Note:        w.Note,
		CoolingDays: w.CoolingDays,
	}
}

func (w *WishlistItem) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	v := w.toValidate()
	if err := sani.SanitizeStruct(v); err != nil {
		return errs.ErrSanitizer
	}
	w.Name, w.URL, w.Note = v.Name, v.URL, v.Note
	valerr := valid.Validate(v)
	if valerr == nil {
		valerr = &errs.ValidationError{Errors: errs.ValidationErrors{}}
	}
	if w.Plan != nil {
		for k, v := range w.Plan.Validate(w.Price) {
			valerr.Errors[k] = v
		}
	}
	if len(valerr.Errors) > 0 {
		return *valerr
	}
	return nil
}

func (w *WishlistItem) IsInstallments() bool {
	return w.Plan != nil
}

func (w *WishlistItem) Purchase() Purchase {
	return Purchase{Price: w.Price, Currency: w.Currency, Plan: w.Plan}
}

// Schedule is its plan worked out, nil when paid cash or the plan doesn't
// work out.
func (w *WishlistItem) Schedule() *InstallmentSchedule {
	if w.Plan == nil {
		return nil
	}
	s, err := w.Plan.Schedule(w.Price)
	if err != nil {
		return nil
	}
	return s
}

// TotalCost is what it costs in the end: the price, or everything its plan
// pays.
func (w *WishlistItem) TotalCost() float64 {
	if s := w.Schedule(); s != nil {
		return s.TotalPaid
	}
	return w.Price
}

// ReadyAt is when the cooling-off period ends.
func (w *WishlistItem) ReadyAt() time.Time {
	return w.CreatedAt.AddDate(0, 0, w.CoolingDays)
}

func (w *WishlistItem) IsReady(now time.Time) bool {
	return !now.Before(w.ReadyAt())
}

// DaysLeft is the whole days until it's ready, rounded up, 0 once ready.
func (w *WishlistItem) DaysLeft(now time.Time) int {
	left := w.ReadyAt().Sub(now)
	if left <= 0 {
		return 0
	}
	return int(math.Ceil(left.Hours() / 24))
}

// CoolingProgress is how much of the cooling-off period has passed, 0 to 1.
func (w *WishlistItem) CoolingProgress(now time.Time) float64 {
	total := w.ReadyAt().Sub(w.CreatedAt)
	if total <= 0 {
		return 1
	}
	return min(1, max(0, float64(now.Sub(w.CreatedAt))/float64(total)))
}

func (w *WishlistItem) ViewPrice() string {
	return w.Currency.Format(w.Price)
}

// ViewPlan sums its plan up, e.g. "12 × E£ 1,100" or "12 mo, E£ 1,200 →
// E£ 850", with the down payment if any. "" when paid cash.
func (w *WishlistItem) ViewPlan() string {
	s := w.Schedule()
	if s == nil {
		return ""
	}
	return s.Summary(w.Currency.Symbol)
}

// WishlistStats sums up the decided items, with what abandoning saved in the
// given currency.
type WishlistStats struct {
	Waiting, Bought, Abandoned int
	Saved                      float64
}

// AbandonRate is the share of decided items that were abandoned, 0 to 100.
func (s WishlistStats) AbandonRate() float64 {
	decided := s.Bought + s.Abandoned
	if decided == 0 {
		return 0
	}
	return float64(s.Abandoned) / float64(decided) * 100
}

func SummarizeWishlist(items []WishlistItem, cur *Currency) WishlistStats {
	var s WishlistStats
	for _, w := range items {
		switch w.Status {
		case WishlistWaiting:
			s.Waiting++
		case WishlistBought:
			s.Bought++
		case WishlistAbandoned:
			s.Abandoned++
			s.Saved += convert(w.TotalCost(), &w.Currency, cur)
		}
	}
	return s
}

// convert converts amount from a currency to another, 0 if the rate is
// unknown.
func convert(amount float64, from, to *Currency) float64 {
	if from.String == to.String {
		return amount
	}
	return amount * from.FxRate(to.String)
}
