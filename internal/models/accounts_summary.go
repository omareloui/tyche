package models

import (
	"cmp"
	"math"
	"slices"
	"strings"
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

// ForecastInfo is what the summary's forecast is made of, in its currency:
// the money each part moves in and out of the counted accounts (net).
type ForecastInfo struct {
	// To is the forecast's last day.
	To time.Time
	// Scheduled is the scheduled transactions', Recurrent the recurrent
	// transactions' occurrences not scheduled yet (RecurrentCount of them),
	// GoalDeposits the goal plans' deposits (GoalDepositCount of them).
	Scheduled, Recurrent, GoalDeposits float64
	RecurrentCount, GoalDepositCount   int
	// BudgetsLeft and BudgetsNext are the budgets spent: what this month's
	// have left, next month's, each less its expenses already scheduled.
	BudgetsLeft, BudgetsNext float64
}

// AccountsSummaryWith are the accounts out of the balance a summary counts
// too (the ones excluded from it, the goals'), and whether its net worth
// leaves out what the month's budgets have left to spend.
type AccountsSummaryWith struct {
	Excluded, Goals bool
	BudgetLeft      bool
	// Months is how many of the user's months the history goes back
	// (AccountHistoryMonths when 0), AllHistory from the first day money
	// moved instead (HistoryFrom, set from the transactions).
	Months      int
	AllHistory  bool
	HistoryFrom time.Time
}

// AccountsSummaryMonths are the ranges the history can be picked over, in
// the user's months.
var AccountsSummaryMonths = []int{3, 6, 12, 24}

// HistoryMonths is how many months the history goes back, 0 for all of it.
func (w AccountsSummaryWith) HistoryMonths() int {
	if w.AllHistory {
		return 0
	}
	if w.Months <= 0 {
		return AccountHistoryMonths
	}
	return w.Months
}

// From is the history's first day: HistoryFrom for all of it (when known),
// else the start of the month HistoryMonths back.
func (w AccountsSummaryWith) From(s MonthStart, today time.Time) time.Time {
	if w.AllHistory && !w.HistoryFrom.IsZero() {
		return s.Of(w.HistoryFrom).From
	}
	return AccountHistoryFromMonths(s, today, w.HistoryMonths())
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
	NetWorth        float64
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
	// Salary is the monthly salary the forecast counts, in Currency, on
	// SalaryOn (0 when not counted); SalaryScheduled tells it's set but
	// already scheduled as an income.
	Salary          float64
	SalaryOn        time.Time
	SalaryScheduled bool
	// Info is what the forecast is made of.
	Info ForecastInfo

	// In is the currency History is also shown in (ShowIn), each past day
	// at its own rates, HistoryIn then; InApprox tells some of those rates
	// were today's instead.
	In        *Currency
	HistoryIn *AccountHistory
	InApprox  bool
	// InRates are the rates from Currency to In each day of HistoryIn up to
	// today was converted at (0 when Currency's own wasn't had), InToday
	// today's.
	InRates []float64
	InToday float64
	// InChoices are the currencies it can be shown in besides Currency.
	InChoices []Currency

	// rates are the counted accounts' rates to Currency, by id, codeRates
	// by currency code (lower case).
	rates     map[string]float64
	codeRates map[string]float64
	// native is History per currency code (lower case), in that currency.
	native map[string][]float64
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
func NewAccountsSummary(accs []Account, def Currency, with AccountsSummaryWith, start MonthStart, today, to time.Time, daily, scheduled map[string][]DayFlow) *AccountsSummary {
	s := &AccountsSummary{Currency: def, With: with, rates: map[string]float64{}, codeRates: map[string]float64{}, native: map[string][]float64{}}
	s.Info.To = dateOnly(to)
	from := with.From(start, today)

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
		s.rates[a.ID] = rate
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

		h := NewAccountHistory(start, a.Amount, daily[a.ID], scheduled[a.ID], from, today, to)
		if s.History == nil {
			s.History = &AccountHistory{From: h.From, Today: h.Today, To: h.To, Start: start, Balance: make([]float64, len(h.Balance)), Months: with.HistoryMonths()}
		}
		code := strings.ToLower(a.Currency.String)
		s.codeRates[code] = rate
		nat := s.native[code]
		if nat == nil {
			nat = make([]float64, len(h.Balance))
			s.native[code] = nat
		}
		for d, b := range h.Balance {
			s.History.Balance[d] += b * rate
			nat[d] += b
		}
	}

	slices.SortStableFunc(s.ByKind, func(a, b KindShare) int { return cmp.Compare(b.Amount, a.Amount) })
	slices.SortStableFunc(s.ByCurrency, func(a, b CurrencyShare) int { return cmp.Compare(b.Amount, a.Amount) })
	return s
}

// Forecast takes the budgets' spending out of the history's projection:
// what cur (today's month) has left spread over the days after today, and
// next's budgets over its days, each category's less its expenses already
// scheduled in the month (part of the projection already).
func (s *AccountsSummary) Forecast(cur, next *BudgetSummary, scheduled []DayCategoryTotal) {
	h := s.History
	if h == nil || !h.HasProjection() {
		return
	}
	t := h.TodayIdx()
	burn := make([]float64, len(h.Balance))
	for k, bs := range []*BudgetSummary{cur, next} {
		if bs == nil {
			continue
		}
		total := &s.Info.BudgetsLeft
		if k == 1 {
			total = &s.Info.BudgetsNext
		}
		first := max(h.Index(bs.Period.From), t+1)
		last := min(h.Index(bs.Period.Last()), len(burn)-1)
		if first > last {
			continue
		}
		for _, it := range bs.Budgeted() {
			left := it.Budget - it.Spent
			for _, sc := range scheduled {
				if sc.CategoryID != it.Category.ID || !bs.Period.Contains(sc.Day) {
					continue
				}
				if amount, ok := Convert(sc.Amount, &sc.Currency, s.Currency.String); ok {
					left -= amount
				}
			}
			if left <= 0 {
				continue
			}
			*total += left
			perDay := left / float64(last-first+1)
			for i := first; i <= last; i++ {
				burn[i] += perDay
			}
		}
	}
	var spent float64
	for i := t + 1; i < len(h.Balance); i++ {
		spent += burn[i]
		h.Balance[i] -= spent
	}
}

// SalaryMatch is how close (a share of it) a scheduled income has to be to
// the salary to take it for the salary.
const SalaryMatch = 0.1

// ForecastSalary adds salary (in Currency) to the history's projection on
// day, unless one of incomes (scheduled in its month) already looks like it.
func (s *AccountsSummary) ForecastSalary(salary float64, day time.Time, incomes []Balance) {
	h := s.History
	if h == nil || salary <= 0 {
		return
	}
	i := h.Index(day)
	if i <= h.TodayIdx() || i >= len(h.Balance) {
		return
	}
	for _, in := range incomes {
		if amount, ok := Convert(in.Amount, &in.Currency, s.Currency.String); ok && math.Abs(amount-salary) <= salary*SalaryMatch {
			s.SalaryScheduled = true
			return
		}
	}
	for ; i < len(h.Balance); i++ {
		h.Balance[i] += salary
	}
	s.Salary, s.SalaryOn = salary, dateOnly(day)
}

// NetFlow is what flows (by account id, in the accounts' currencies) move
// in and out of the counted accounts, net, in Currency.
func (s *AccountsSummary) NetFlow(flows map[string][]DayFlow) float64 {
	var net float64
	for id, days := range flows {
		rate, ok := s.rates[id]
		if !ok {
			continue
		}
		for _, d := range days {
			net += (d.In - d.Out) * rate
		}
	}
	return net
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

// historyRateDays is the gap between the days ShowIn takes the rates of.
const historyRateDays = 7

// historyRateSamples is how many days' rates ShowIn takes at most.
const historyRateSamples = 60

// Codes are the counted accounts' currency codes (lower case), sorted.
func (s *AccountsSummary) Codes() []string {
	codes := make([]string, 0, len(s.native))
	for c := range s.native {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	return codes
}

// RateDays are the days whose rates ShowIn converts the history with: one a
// week from its first day, and today.
func (s *AccountsSummary) RateDays() []time.Time {
	h := s.History
	if h == nil {
		return nil
	}
	// A week apart, or wider over long ranges (historyRateSamples at most).
	step := max(historyRateDays, (h.TodayIdx()+historyRateSamples-1)/historyRateSamples)
	var days []time.Time
	for d := h.From; d.Before(h.Today); d = d.AddDate(0, 0, step) {
		days = append(days, d)
	}
	return append(days, h.Today)
}

// ShowIn sets HistoryIn: History in cur, each day up to today at the rates
// (rates[day][code], to cur) of the last of days (RateDays, oldest first) on
// or before it, a currency without one there at today's rates (approx
// then); the forecast's days at todayRate, from Currency to cur, as the
// forecast is in today's money.
func (s *AccountsSummary) ShowIn(cur Currency, days []time.Time, rates map[time.Time]map[string]float64, todayRate float64, approx bool) {
	h := s.History
	if h == nil || len(days) == 0 {
		return
	}
	s.In = &cur
	s.InApprox = approx
	in := &AccountHistory{From: h.From, Today: h.Today, To: h.To, Start: h.Start, Balance: make([]float64, len(h.Balance)), Months: h.Months}
	s.HistoryIn = in

	t := h.TodayIdx()
	s.InToday = todayRate
	s.InRates = make([]float64, min(t+1, len(in.Balance)))
	defCode := strings.ToLower(s.Currency.String)
	k := 0
	for i := range in.Balance {
		if i > t {
			in.Balance[i] = h.Balance[i] * todayRate
			continue
		}
		day := h.DayOf(i)
		for k+1 < len(days) && !dateOnly(days[k+1]).After(day) {
			k++
		}
		s.InRates[i] = rates[days[k]][defCode]
		for code, nat := range s.native {
			if i >= len(nat) {
				continue
			}
			rate, ok := rates[days[k]][code]
			if !ok || rate <= 0 {
				rate = s.codeRates[code] * todayRate
				s.InApprox = true
			}
			in.Balance[i] += nat[i] * rate
		}
	}
}

// Change is how much a history's balance moved from its first day to
// today's, relative to the first; ok is false when it started at 0.
func (h *AccountHistory) Change() (float64, bool) {
	if h == nil || len(h.Balance) == 0 {
		return 0, false
	}
	first, now := h.Balance[0], h.Balance[min(h.TodayIdx(), len(h.Balance)-1)]
	if first == 0 {
		return 0, false
	}
	return (now - first) / math.Abs(first), true
}

// InDay is HistoryIn's i-th day (up to today) told in Currency: the net
// worth that day at that day's rates (HistoryIn's balance back at the rate
// of Currency then), what one unit of In (a gram or an ounce for a metal)
// was worth in Currency then and today, and how much that moved since; ok is
// false without that day's rate.
func (s *AccountsSummary) InDay(i int) (worth, unitThen, unitNow, change float64, ok bool) {
	if s.HistoryIn == nil || i < 0 || i >= len(s.InRates) || s.InRates[i] <= 0 || s.InToday <= 0 {
		return 0, 0, 0, 0, false
	}
	rate := s.InRates[i]
	worth = s.HistoryIn.Balance[i] / rate
	// Rates are per troy ounce for a metal: per its unit shown.
	perUnit := ToUnit(1, s.In.Unit())
	unitThen = 1 / rate / perUnit
	unitNow = 1 / s.InToday / perUnit
	return worth, unitThen, unitNow, (unitNow - unitThen) / unitThen, true
}
