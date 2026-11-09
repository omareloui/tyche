package models

import (
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// Goal is something to save for. Its money sits in its own hidden accounts
// (AccKindGoal): Account in its currency and a holding per other currency
// money was put in with, so that money keeps its value in its currency and
// the goal is worth it at today's rates. Putting money towards it is a
// transfer to the account of the money's currency, taking some back one
// from it, and buying it an expense out of Account, its holdings converted
// to it first.
type Goal struct {
	ID       string  `validate:"required"`
	Name     string  `validate:"required" conform:"trim,titlecase"`
	Price    float64 `validate:"required,gt=0"`
	Currency Currency
	// Account holds the money saved in the goal's currency.
	Account *Account
	// Holdings hold the money saved in other currencies, one account each.
	Holdings []Account
	// Purchased is what buying it took out of its account.
	Purchased    float64
	FulfilledAt  time.Time
	CreatedAt    time.Time
	Transactions []Transaction
	// Plan is how it's saved for, nil without one.
	Plan *GoalPlan
	// Loan is the payments of the loan it was bought with, in its currency.
	Loan GoalLoan
}

// GoalLoan is the payments of the loan a goal was bought with.
type GoalLoan struct {
	Payments, PaidPayments int
	Total, Paid            float64
}

// HasLoan tells if it was bought with a loan, its payments scheduled.
func (gl *Goal) HasLoan() bool {
	return gl.Loan.Payments > 0
}

// PayingOff tells if it's bought with a loan not paid off yet.
func (gl *Goal) PayingOff() bool {
	return !gl.FulfilledAt.IsZero() && gl.HasLoan() && gl.Loan.PaidPayments < gl.Loan.Payments
}

// PayoffPercentage is how much of what buying it costs is paid: what it
// took on the day and the loan's payments made, of those and the rest.
func (gl *Goal) PayoffPercentage() float64 {
	total := gl.Purchased + gl.Loan.Total
	if total <= 0 {
		return 0
	}
	return (gl.Purchased + gl.Loan.Paid) / total * 100
}

// ShownPercentage is the saving progress, or the paying off once bought
// with a loan.
func (gl *Goal) ShownPercentage() float64 {
	if !gl.FulfilledAt.IsZero() && gl.HasLoan() {
		return gl.PayoffPercentage()
	}
	return gl.ProgressPercentage()
}

// GoalAccountPrefix starts the name of the goals' accounts.
const GoalAccountPrefix = "Goal: "

// GoalAccountName is the name of the goal's (named name) account holding
// currency cur, its own or a holding.
func GoalAccountName(name, cur string) string {
	return GoalAccountPrefix + name + " (" + strings.ToUpper(cur) + ")"
}

type createGoal struct {
	ID         string  `validate:"required"`
	Name       string  `validate:"required" conform:"trim,titlecase"`
	Price      float64 `validate:"required,gt=0"`
	CurrencyID string  `validate:"required"`
}

func (gl *Goal) ViewAmount() string {
	return gl.Currency.Format(gl.Price)
}

// TxAmount is what tx put towards it (negative when taken out), in its
// currency at today's rate: conversions between its accounts and the loan's
// payments put nothing.
func (gl *Goal) TxAmount(tx *Transaction) float64 {
	from := gl.OwnsAccount(tx.Account.ID)
	to := tx.ToAccount != nil && gl.OwnsAccount(tx.ToAccount.ID)
	switch {
	case from && to:
		return 0
	case to:
		return gl.ValueOf(tx.ToAmount, &tx.ToAccount.Currency)
	case from:
		return -gl.ValueOf(tx.Amount, &tx.Account.Currency)
	}
	return 0
}

// IsConversion tells if tx moves money between its accounts (a holding
// converted to its currency when buying it).
func (gl *Goal) IsConversion(tx *Transaction) bool {
	return tx.ToAccount != nil && gl.OwnsAccount(tx.Account.ID) && gl.OwnsAccount(tx.ToAccount.ID)
}

// ScheduledDeposits is what its scheduled (not yet fulfilled) transactions
// will put in it.
func (gl *Goal) ScheduledDeposits() float64 {
	var sum float64
	for _, tx := range gl.Transactions {
		if amt := gl.TxAmount(&tx); tx.FulfilledAt.IsZero() && amt > 0 {
			sum += amt
		}
	}
	return sum
}

// PaidSince is what was put in it since from (net of what was taken out,
// scheduled deposits counted as made), in its currency at today's rates.
func (gl *Goal) PaidSince(from time.Time) float64 {
	var sum float64
	for _, tx := range gl.Transactions {
		if tx.ScheduledTo.Before(from) {
			continue
		}
		if amt := gl.TxAmount(&tx); !tx.FulfilledAt.IsZero() || amt > 0 {
			sum += amt
		}
	}
	return sum
}

// PlanBaseline is what it held before the user's month (starting on start)
// startsOn is in, at today's rates: what a plan starting then counts its
// deposits from, so a rate moving doesn't read as money put in or out.
func (gl *Goal) PlanBaseline(start MonthStart, startsOn time.Time) float64 {
	return gl.Progress() + gl.ScheduledDeposits() - gl.PaidSince(start.Of(startsOn).From)
}

// ProjectPlan works its plan out from its progress, scheduled deposits
// counted as made, from its baseline.
func (gl *Goal) ProjectPlan(start MonthStart, now time.Time) (*GoalProjection, error) {
	p := *gl.Plan
	p.StartSaved = gl.PlanBaseline(start, p.StartsOn)
	return p.Project(gl.Price, gl.Progress()+gl.ScheduledDeposits(), start, now)
}

// Progress is the money saved, in its currency: its accounts' balances at
// today's rates plus what buying it took out of it.
func (gl *Goal) Progress() float64 {
	return gl.Balance() + gl.Purchased
}

// Balance is the money still in its accounts (left after buying it), in its
// currency at today's rates. A holding without a rate counts as 0.
func (gl *Goal) Balance() float64 {
	var sum float64
	if gl.Account != nil {
		sum = gl.Account.Amount
	}
	for i := range gl.Holdings {
		sum += convert(gl.Holdings[i].Amount, &gl.Holdings[i].Currency, &gl.Currency)
	}
	return sum
}

func (gl *Goal) ViewBalance() string {
	return gl.Currency.Format(gl.Balance())
}

// HasHoldings tells if money in other currencies is in it.
func (gl *Goal) HasHoldings() bool {
	return slices.ContainsFunc(gl.Holdings, func(a Account) bool { return RoundCents(a.Amount) > 0 })
}

// Funded are its accounts with money in them, its own first.
func (gl *Goal) Funded() []Account {
	var res []Account
	if gl.Account != nil && RoundCents(gl.Account.Amount) > 0 {
		res = append(res, *gl.Account)
	}
	for _, h := range gl.Holdings {
		if RoundCents(h.Amount) > 0 {
			res = append(res, h)
		}
	}
	return res
}

// AccountIn is its account holding currency cur, nil without one.
func (gl *Goal) AccountIn(cur string) *Account {
	if gl.Account != nil && gl.Account.Currency.String == cur {
		return gl.Account
	}
	for i := range gl.Holdings {
		if gl.Holdings[i].Currency.String == cur {
			return &gl.Holdings[i]
		}
	}
	return nil
}

// AccountByID is its account of id, its own or a holding, nil when it's not
// one of them.
func (gl *Goal) AccountByID(id string) *Account {
	if gl.Account != nil && gl.Account.ID == id {
		return gl.Account
	}
	for i := range gl.Holdings {
		if gl.Holdings[i].ID == id {
			return &gl.Holdings[i]
		}
	}
	return nil
}

// OwnsAccount tells if the account of id is one of its accounts.
func (gl *Goal) OwnsAccount(id string) bool {
	return gl.AccountByID(id) != nil
}

// ValueOf is amount of cur in its currency at today's rate, 0 without one.
func (gl *Goal) ValueOf(amount float64, cur *Currency) float64 {
	return convert(amount, cur, &gl.Currency)
}

func (gl *Goal) ProgressPercentage() float64 {
	return gl.Progress() / gl.Price * 100
}

func (gl *Goal) Remaining() float64 {
	return gl.Price - gl.Progress()
}

func (gl *Goal) ViewRemaining() string {
	return gl.Currency.Format(gl.Remaining())
}

func (gl *Goal) ViewProgress() string {
	return gl.Currency.Format(gl.Progress())
}

// MonthlyFor is what to put aside every month to get there in
// numberOfMonths.
func (gl *Goal) MonthlyFor(numberOfMonths int) float64 {
	return numsutils.Round(gl.Remaining() / float64(numberOfMonths))
}

func (gl *Goal) toValidate() *createGoal {
	return &createGoal{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	}
}

func (gl *Goal) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	glToValidate := gl.toValidate()
	if err := sani.SanitizeStruct(glToValidate); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(glToValidate); valerr != nil {
		return *valerr
	}
	return nil
}
