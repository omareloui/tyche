package models

import (
	"slices"
	"testing"
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
	s := NewAccountsSummary(accs, egp, AccountsSummaryWith{}, MonthStart(1), today, daily)

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
	s := NewAccountsSummary(accs, egp, AccountsSummaryWith{Excluded: true}, MonthStart(1), today, nil)
	if s.Accounts != 2 || s.NetWorth != 6000 || s.SetAside != 700 {
		t.Errorf("with excluded: accounts = %d, net worth = %v, set aside = %v", s.Accounts, s.NetWorth, s.SetAside)
	}
	// The goal's account holds what's set aside, so it isn't added again.
	s = NewAccountsSummary(accs, egp, AccountsSummaryWith{Goals: true}, MonthStart(1), today, nil)
	if s.Accounts != 2 || s.NetWorth != 1700 || s.SetAside != 0 || s.ByKind[1].Kind != AccKindGoal {
		t.Errorf("with goals: accounts = %d, net worth = %v, set aside = %v, kinds = %+v", s.Accounts, s.NetWorth, s.SetAside, s.ByKind)
	}
}

func TestNewAccountsSummaryEmpty(t *testing.T) {
	s := NewAccountsSummary(nil, Currency{String: "EGP"}, AccountsSummaryWith{}, MonthStart(1), day("2026-11-05"), nil)
	if s.Accounts != 0 || s.History != nil || s.HasCredit() || s.Share(10) != 0 {
		t.Errorf("summary = %+v", s)
	}
}
