package models

import (
	"cmp"
	"slices"
	"time"
)

// KindShare is what the accounts of a kind hold, in the summary's currency.
type KindShare struct {
	Kind     AccKind
	Amount   float64
	Accounts int
}

// CurrencyShare is what the accounts in a currency hold, in it (Native) and
// in the summary's currency (Amount), Rate being one unit's worth.
type CurrencyShare struct {
	Currency Currency
	Native   float64
	Amount   float64
	Rate     float64
}

// CardsSummary is the credit cards' debt against their limits, in the
// summary's currency.
type CardsSummary struct {
	Cards int
	Owed  float64
	Limit float64
	// NextDue is the card whose last statement is due first, if any is left
	// to pay.
	NextDue *Account
}

// UsedPct is the share of the limits owed.
func (c *CardsSummary) UsedPct() float64 {
	if c.Limit <= 0 {
		return 0
	}
	return min(100, c.Owed/c.Limit*100)
}

// AccountsSummaryWith are the accounts out of the balance a summary counts
// too (the ones excluded from it, the goals'), and whether its net worth
// leaves out what the month's budgets have left to spend.
type AccountsSummaryWith struct {
	Excluded, Goals bool
	BudgetLeft      bool
}

// AccountsSummary is the accounts counted in the balance (not archived nor
// excluded from it, plus the ones With asks for): their net worth, what they hold per kind and currency,
// the net worth day by day over the last months and the cards' and debts'
// state, in Currency. Amounts are converted at today's rates.
type AccountsSummary struct {
	Currency Currency
	With     AccountsSummaryWith
	// Excluded and Goals count the accounts excluded from the balance and
	// the goals', counted or not.
	Excluded, Goals int
	Accounts        int
	NetWorth float64
	// Assets are the positive balances, Owed the negative ones (positive).
	Assets, Owed float64
	// SetAside is what the accounts moved to goals in progress, out of the
	// balance; 0 when counting the goals' accounts, which hold it.
	SetAside   float64
	ByKind     []KindShare
	ByCurrency []CurrencyShare
	// History is the net worth worked back day by day, Balance summing every
	// account's at today's rates.
	History *AccountHistory
	Cards   CardsSummary
	// OwedToYou and YouOwe are the debt accounts' positive and negative
	// balances (both positive).
	OwedToYou, YouOwe float64
	Partial           bool
	// BudgetLeft is what the current month's budgets have left to spend,
	// whether With.BudgetLeft or not.
	BudgetLeft float64
}

// Shown is the net worth shown: less the budgets' left when asked.
func (s *AccountsSummary) Shown() float64 {
	if s.With.BudgetLeft {
		return s.NetWorth - s.BudgetLeft
	}
	return s.NetWorth
}

// NewAccountsSummary sums accs (the live ones and the goals', card
// statements attached) in def, their balances worked back from today through daily, the money each
// moved per day (by account id) since AccountHistoryFrom.
func NewAccountsSummary(accs []Account, def Currency, with AccountsSummaryWith, start MonthStart, today time.Time, daily map[string][]DayFlow) *AccountsSummary {
	s := &AccountsSummary{Currency: def, With: with}
	from := AccountHistoryFrom(start, today)

	kinds := map[AccKind]int{}
	curs := map[string]int{}
	for i := range accs {
		a := &accs[i]
		if a.IsArchive || a.IsDeleted() {
			continue
		}
		if a.Kind.Enum == AccKindGoal {
			if s.Goals++; !with.Goals {
				continue
			}
		} else if a.ExcludeFromBalance {
			if s.Excluded++; !with.Excluded {
				continue
			}
		}
		rate, ok := Convert(1, &a.Currency, def.String)
		if !ok {
			s.Partial = true
			continue
		}
		s.Accounts++
		amount := a.Amount * rate
		s.NetWorth += amount
		if !with.Goals {
			s.SetAside += a.SetAside * rate
		}
		if amount < 0 {
			s.Owed -= amount
		} else {
			s.Assets += amount

			k, ok := kinds[a.Kind.Enum]
			if !ok {
				k = len(s.ByKind)
				kinds[a.Kind.Enum] = k
				s.ByKind = append(s.ByKind, KindShare{Kind: a.Kind.Enum})
			}
			s.ByKind[k].Amount += amount
			s.ByKind[k].Accounts++

			c, ok := curs[a.Currency.String]
			if !ok {
				c = len(s.ByCurrency)
				curs[a.Currency.String] = c
				s.ByCurrency = append(s.ByCurrency, CurrencyShare{Currency: a.Currency, Rate: rate})
			}
			s.ByCurrency[c].Native += a.Amount
			s.ByCurrency[c].Amount += amount
		}

		switch a.Kind.Enum {
		case AccKindCreditCard:
			s.Cards.Cards++
			s.Cards.Owed += max(0, -amount)
			s.Cards.Limit += a.CardLimit * rate
			if st := a.Statement; st != nil && st.Due > 0 &&
				(s.Cards.NextDue == nil || st.DueOn.Before(s.Cards.NextDue.Statement.DueOn)) {
				s.Cards.NextDue = a
			}
		case AccKindDebt:
			if amount > 0 {
				s.OwedToYou += amount
			} else {
				s.YouOwe -= amount
			}
		}

		h := NewAccountHistory(start, a.Amount, daily[a.ID], nil, from, today, today)
		if s.History == nil {
			s.History = &AccountHistory{From: h.From, Today: h.Today, To: h.To, Start: start, Balance: make([]float64, len(h.Balance))}
		}
		for d, b := range h.Balance {
			s.History.Balance[d] += b * rate
		}
	}

	slices.SortStableFunc(s.ByKind, func(a, b KindShare) int { return cmp.Compare(b.Amount, a.Amount) })
	slices.SortStableFunc(s.ByCurrency, func(a, b CurrencyShare) int { return cmp.Compare(b.Amount, a.Amount) })
	return s
}

// Share is the amount's percentage of the assets.
func (s *AccountsSummary) Share(amount float64) float64 {
	if s.Assets <= 0 {
		return 0
	}
	return max(amount, 0) / s.Assets * 100
}

// HasCredit tells whether there are cards or debts to show.
func (s *AccountsSummary) HasCredit() bool {
	return s.Cards.Cards > 0 || s.OwedToYou > 0 || s.YouOwe > 0
}
