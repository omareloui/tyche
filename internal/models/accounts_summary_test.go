package models

import (
	"slices"
	"testing"
	"time"
)

func TestNewAccountsSummary(t *testing.T) {
	egp := Currency{String: "EGP"}
	xyz := Currency{String: "XYZ"} // no rate
	today := day("2026-11-05")
	kind := func(k AccKind) AccountKind { return AccountKind{Enum: k} }
	accs := []Account{
		{ID: "bank", Amount: 1000, Currency: egp, Kind: kind(AccKindBankAcc), SetAside: 200},
		{ID: "cash", Amount: 300, Currency: egp, Kind: kind(AccKindCash)},
		{ID: "card", Amount: -400, Currency: egp, Kind: kind(AccKindCreditCard), CardLimit: 1000, Statement: &CardStatement{Due: 150, DueOn: day("2026-11-20")}},
		{ID: "card2", Amount: 0, Currency: egp, Kind: kind(AccKindCreditCard), CardLimit: 1000, Statement: &CardStatement{Due: 50, DueOn: day("2026-11-10")}},
		{ID: "lent", Amount: 250, Currency: egp, Kind: kind(AccKindDebt)},
		{ID: "borrowed", Amount: -100, Currency: egp, Kind: kind(AccKindDebt)},
		{ID: "old", Amount: 5000, Currency: egp, Kind: kind(AccKindBankAcc), IsArchive: true},
		{ID: "hidden", Amount: 5000, Currency: egp, Kind: kind(AccKindBankAcc), ExcludeFromBalance: true},
		{ID: "gone", Amount: 5000, Currency: egp, Kind: kind(AccKindBankAcc), DeletedAt: today},
		{ID: "odd", Amount: 5, Currency: xyz, Kind: kind(AccKindCash)},
		{ID: "car", Amount: 700, Currency: egp, Kind: kind(AccKindGoal)},
	}
	daily := map[string][]DayFlow{
		"bank": {{Day: day("2026-11-01"), In: 600}},
		"cash": {{Day: day("2026-11-03"), Out: 100}},
		"old":  {{Day: day("2026-11-02"), In: 9999}},
	}
	s := NewAccountsSummary(accs, egp, AccountsSummaryWith{}, MonthStart(1), today, today, daily, nil)

	if s.Accounts != 6 || !s.Partial || s.Excluded != 1 || s.Goals != 1 {
		t.Errorf("accounts = %d, partial = %v", s.Accounts, s.Partial)
	}
	if s.NetWorth != 1050 || s.Assets != 1550 || s.Owed != 500 || s.SetAside != 200 {
		t.Errorf("net worth = %v, assets = %v, owed = %v, set aside = %v", s.NetWorth, s.Assets, s.Owed, s.SetAside)
	}

	var kinds []AccKind
	for _, k := range s.ByKind {
		kinds = append(kinds, k.Kind)
	}
	if want := []AccKind{AccKindBankAcc, AccKindCash, AccKindDebt, AccKindCreditCard}; !slices.Equal(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if len(s.ByCurrency) != 1 || s.ByCurrency[0].Native != 1550 {
		t.Errorf("currencies = %+v", s.ByCurrency)
	}

	if c := s.Cards; c.Cards != 2 || c.Owed != 400 || c.Limit != 2000 || c.UsedPct() != 20 || c.NextDue == nil || c.NextDue.ID != "card2" {
		t.Errorf("cards = %+v", c)
	}
	if s.OwedToYou != 250 || s.YouOwe != 100 || !s.HasCredit() {
		t.Errorf("owed to you = %v, you owe = %v", s.OwedToYou, s.YouOwe)
	}

	h := s.History
	if h == nil || !h.From.Equal(AccountHistoryFrom(MonthStart(1), today)) {
		t.Fatalf("history = %+v", h)
	}
	at := func(d string) float64 { return h.Balance[daysFrom(h.From, day(d))] }
	// Worked back: the bank had 600 less before Nov 1, the cash 100 more
	// before Nov 3; the archived account's money is left out.
	if at("2026-11-05") != 1050 || at("2026-11-02") != 1150 || at("2026-10-31") != 550 {
		t.Errorf("history = %v / %v / %v, want 1050 / 1150 / 550", at("2026-11-05"), at("2026-11-02"), at("2026-10-31"))
	}
}

func TestNewAccountsSummaryWith(t *testing.T) {
	egp := Currency{String: "EGP"}
	accs := []Account{
		{ID: "bank", Amount: 1000, Currency: egp, Kind: AccountKind{Enum: AccKindBankAcc}, SetAside: 700},
		{ID: "savings", Amount: 5000, Currency: egp, Kind: AccountKind{Enum: AccKindBankAcc}, ExcludeFromBalance: true},
		{ID: "car", Amount: 700, Currency: egp, Kind: AccountKind{Enum: AccKindGoal}},
	}
	today := day("2026-11-05")
	s := NewAccountsSummary(accs, egp, AccountsSummaryWith{Excluded: true}, MonthStart(1), today, today, nil, nil)
	if s.Accounts != 2 || s.NetWorth != 6000 || s.SetAside != 700 {
		t.Errorf("with excluded: accounts = %d, net worth = %v, set aside = %v", s.Accounts, s.NetWorth, s.SetAside)
	}
	// The goal's account holds what's set aside, so it isn't added again.
	s = NewAccountsSummary(accs, egp, AccountsSummaryWith{Goals: true}, MonthStart(1), today, today, nil, nil)
	if s.Accounts != 2 || s.NetWorth != 1700 || s.SetAside != 0 || s.ByKind[1].Kind != AccKindGoal {
		t.Errorf("with goals: accounts = %d, net worth = %v, set aside = %v, kinds = %+v", s.Accounts, s.NetWorth, s.SetAside, s.ByKind)
	}
}

func TestAccountsSummaryShown(t *testing.T) {
	s := &AccountsSummary{NetWorth: 1000, BudgetLeft: 300}
	if s.Shown() != 1000 {
		t.Errorf("shown = %v, want 1000", s.Shown())
	}
	s.With.BudgetLeft = true
	if s.Shown() != 700 {
		t.Errorf("shown less the budget left = %v, want 700", s.Shown())
	}
}

func TestNewAccountsSummaryEmpty(t *testing.T) {
	s := NewAccountsSummary(nil, Currency{String: "EGP"}, AccountsSummaryWith{}, MonthStart(1), day("2026-11-05"), day("2026-11-05"), nil, nil)
	if s.Accounts != 0 || s.History != nil || s.HasCredit() || s.Share(10) != 0 {
		t.Errorf("summary = %+v", s)
	}
}

func TestAccountsSummaryForecast(t *testing.T) {
	egp := Currency{String: "EGP"}
	today, end := day("2026-11-05"), day("2026-12-31")
	start := MonthStart(1)
	accs := []Account{{ID: "bank", Amount: 1000, Currency: egp, Kind: AccountKind{Enum: AccKindBankAcc}}}
	scheduled := map[string][]DayFlow{"bank": {
		{Day: day("2026-11-20"), Out: 300}, // rent
		{Day: day("2026-12-01"), In: 2000}, // salary
	}}
	s := NewAccountsSummary(accs, egp, AccountsSummaryWith{BudgetLeft: true}, start, today, end, nil, scheduled)

	cats := []Category{
		{ID: "food", BudgetAmount: 260, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
		{ID: "rent", BudgetAmount: 300, BudgetCurrency: &egp, Kinds: []TxKind{ExpenseTxKind}},
	}
	nov, dec := start.Of(today), start.Of(today).Next()
	cur := NewBudgetSummary(nov, today, egp, cats, []DayCategoryTotal{
		{day("2026-11-02"), CategoryTotal{CategoryID: "food", Currency: egp, Amount: 100}},
	}, nil)
	next := NewBudgetSummary(dec, today, egp, cats, nil, nil)
	// The rent is scheduled in November, so its budget isn't spent again.
	s.Forecast(cur, next, []DayCategoryTotal{
		{day("2026-11-20"), CategoryTotal{CategoryID: "rent", Currency: egp, Amount: 300}},
	})

	if !near(s.Info.BudgetsLeft, 160) || !near(s.Info.BudgetsNext, 560) {
		t.Errorf("budgets left = %v, next = %v, want 160 and 560", s.Info.BudgetsLeft, s.Info.BudgetsNext)
	}
	if got := s.NetFlow(scheduled); got != 1700 {
		t.Errorf("scheduled net = %v, want 1700", got)
	}

	h := s.History
	at := func(d string) float64 { return h.Balance[h.Index(day(d))] }
	if !h.HasProjection() || at("2026-11-05") != 1000 {
		t.Fatalf("today = %v, projected = %v", at("2026-11-05"), h.HasProjection())
	}
	// Food's 160 left over Nov 6–30, the rent paid: 1000 − 300 − 160.
	if !near(at("2026-11-30"), 540) {
		t.Errorf("Nov 30 = %v, want 540", at("2026-11-30"))
	}
	// Then the salary, and December's food and rent budgets.
	if !near(at("2026-12-31"), 540+2000-260-300) {
		t.Errorf("Dec 31 = %v, want %v", at("2026-12-31"), 540+2000-260-300)
	}
}

func TestAccountsSummaryForecastSalary(t *testing.T) {
	egp := Currency{String: "EGP"}
	today, end := day("2026-11-05"), day("2026-12-24")
	// Months start on the 25th: the next one on Nov 25.
	newSummary := func() *AccountsSummary {
		accs := []Account{{ID: "bank", Amount: 1000, Currency: egp, Kind: AccountKind{Enum: AccKindBankAcc}}}
		return NewAccountsSummary(accs, egp, AccountsSummaryWith{BudgetLeft: true}, MonthStart(25), today, end, nil, nil)
	}

	s := newSummary()
	s.ForecastSalary(5000, day("2026-11-25"), nil)
	at := func(d string) float64 { return s.History.Balance[s.History.Index(day(d))] }
	if at("2026-11-24") != 1000 || at("2026-11-25") != 6000 || at("2026-12-24") != 6000 || !s.SalaryOn.Equal(day("2026-11-25")) {
		t.Errorf("balances = %v / %v / %v, salary on %v", at("2026-11-24"), at("2026-11-25"), at("2026-12-24"), s.SalaryOn)
	}

	// A scheduled income close to it is taken for it.
	s = newSummary()
	s.ForecastSalary(5000, day("2026-11-25"), []Balance{{Amount: 200, Currency: egp}, {Amount: 4800, Currency: egp}})
	if s.Salary != 0 || s.History.Balance[len(s.History.Balance)-1] != 1000 {
		t.Errorf("salary counted again: %v", s.Salary)
	}
}

func TestRecurrentOccurrencesBetween(t *testing.T) {
	rt := &RecurrentTx{Unit: RecurrentUnitMonth, Every: 1, LastTransactionScheduledTo: day("2026-10-31")}
	got := rt.OccurrencesBetween(day("2026-11-05"), day("2027-01-01"))
	// The 31st falls on the months' last days, the one before after skipped.
	if want := []time.Time{day("2026-11-30"), day("2026-12-30")}; !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("monthly = %v, want %v", got, want)
	}

	rt = &RecurrentTx{Unit: RecurrentUnitDay, Every: 7, LastTransactionScheduledTo: day("2026-10-01"), StopsAt: day("2026-11-20")}
	got = rt.OccurrencesBetween(day("2026-11-05"), day("2027-01-01"))
	if want := []time.Time{day("2026-11-12"), day("2026-11-19")}; !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("weekly until it stops = %v, want %v", got, want)
	}

	if got := (&RecurrentTx{Unit: RecurrentUnitDay}).OccurrencesBetween(day("2026-11-05"), day("2027-01-01")); got != nil {
		t.Errorf("every 0 = %v", got)
	}
}

func TestPlannedDeposits(t *testing.T) {
	egp := Currency{String: "EGP"}
	now := day("2026-11-05")
	bank := Account{ID: "bank", Amount: 10000, Currency: egp, Kind: AccountKind{Enum: AccKindBankAcc}, IsDefault: true}
	goals := []Goal{
		{
			ID: "car", Price: 1000, Currency: egp,
			Account: &Account{ID: "car-acc", Currency: egp, Kind: AccountKind{Enum: AccKindGoal}},
			Plan:    &GoalPlan{Mode: GoalPlanSave, Monthly: 300, StartsOn: day("2026-11-01")},
		},
		// No plan, bought: nothing asked.
		{ID: "tv", Price: 500, Currency: egp, Account: &Account{ID: "tv-acc", Currency: egp}},
		{ID: "phone", Price: 500, Currency: egp, FulfilledAt: now, Account: &Account{ID: "ph-acc", Currency: egp}, Plan: &GoalPlan{Monthly: 100, StartsOn: now}},
	}
	got := PlannedDeposits(goals, []Account{bank}, MonthStart(1), now, day("2027-01-01"))
	// November's (due now) on the next payday, Dec 1, as December's.
	if len(got) != 2 || !got[0].On.Equal(day("2026-12-01")) || !got[1].On.Equal(day("2026-12-01")) || got[0].Amount != 300 || got[1].Source.ID != "bank" || got[1].SourceTotal != 300 {
		t.Fatalf("deposits = %+v", got)
	}

	// Months starting on the 26th: November's on Nov 26, December's (asked
	// on Dec 1) on Dec 26, after the forecast's end.
	got = PlannedDeposits(goals, []Account{bank}, MonthStart(26), now, day("2026-12-26"))
	if len(got) != 1 || !got[0].On.Equal(day("2026-11-26")) {
		t.Errorf("deposits from the 26th = %+v", got)
	}
}
