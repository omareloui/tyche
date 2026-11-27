package txguess

import (
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func testIn() In {
	return In{
		Text:     "x",
		Today:    time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		Timezone: "Africa/Cairo",
		Accounts: []Account{
			{ID: "acc-cib", Name: "CIB", Currency: "EGP", Kind: "Bank account"},
			{ID: "acc-cash", Name: "Cash", Currency: "EGP", Kind: "Cash"},
		},
		Categories: []Category{
			{ID: "cat-food", Name: "Food", Kinds: []models.TxKind{models.ExpenseTxKind}},
			{ID: "cat-salary", Name: "Salary", Kinds: []models.TxKind{models.IncomeTxKind}},
		},
		Counterparties: []Counterparty{
			{ID: "cp-shop", Name: "Goods Shop", IsMerchant: true, Aliases: []string{"GOODS*12"}},
			{ID: "cp-ahmed", Name: "Ahmed Ali"},
		},
	}
}

func TestPrompt(t *testing.T) {
	p := Prompt(testIn())
	for _, want := range []string{"A1: CIB, EGP", "C2: Salary (income)", "P1: Goods Shop, business | GOODS*12", "P2: Ahmed Ali, person", "2026-11-27 (Friday)"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, id := range []string{"acc-cib", "cat-food", "cp-shop"} {
		if strings.Contains(p, id) {
			t.Errorf("prompt has the id %q", id)
		}
	}
}

func TestParse(t *testing.T) {
	in := testIn()
	got, skipped := Parse(in, []Answer{
		{Kind: "expense", Amount: 85, Account: "A1", Category: "C1", Date: "2026-11-26", Text: "coffee 85 cib yesterday"},
		// invalid refs dropped, a new name like an alias is a match
		{Kind: "expense", Amount: 120, Account: "A9", Category: "C7", NewCounterparty: "goods*12", NewIsMerchant: true, Time: "13:05"},
		// a category of another kind and a counterparty on a transfer dropped
		{Kind: "transfer", Amount: 500, Account: "A1", ToAccount: "A2", Category: "C1", Counterparty: "P1"},
		{Kind: "income", Amount: 40000, Category: "C1", Counterparty: "P9", NewCounterparty: "Acme Corp", NewIsMerchant: true},
		{Kind: "lend", Amount: 500, Counterparty: "P2", Category: "C1"},
		// skipped
		{Kind: "expense", Amount: 0, Text: "lunch"},
		{Kind: "transfer", Amount: 10, Account: "A1", ToAccount: "A1", Text: "to myself"},
		{Kind: "borrow", Amount: 10, Counterparty: "P1", Text: "from the shop"},
		{Kind: "gift", Amount: 10},
	})
	if len(got) != 5 {
		t.Fatalf("got %d guesses: %+v", len(got), got)
	}
	if g := got[0]; g.AccountID != "acc-cib" || g.CategoryID != "cat-food" || !g.Day.Equal(time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)) || g.HasTime {
		t.Errorf("first = %+v", g)
	}
	if g := got[1]; g.AccountID != "" || g.CategoryID != "" || g.CounterpartyID != "cp-shop" || g.NewCounterparty != "" || !g.HasTime || g.Clock != 13*time.Hour+5*time.Minute || !g.Day.IsZero() {
		t.Errorf("second = %+v", g)
	}
	if g := got[2]; g.ToAccountID != "acc-cash" || g.CategoryID != "" || g.CounterpartyID != "" {
		t.Errorf("transfer = %+v", g)
	}
	if g := got[3]; g.CategoryID != "" || g.CounterpartyID != "" || g.NewCounterparty != "Acme Corp" || !g.NewIsMerchant {
		t.Errorf("income = %+v", g)
	}
	if g := got[4]; g.Kind.TxKind() != models.DebtTxKind || g.CounterpartyID != "cp-ahmed" || g.CategoryID != "" {
		t.Errorf("lend = %+v", g)
	}
	if len(skipped) != 4 {
		t.Errorf("skipped = %+v", skipped)
	}
	if skipped[0].Text != "lunch" || skipped[0].Reason != "No amount" {
		t.Errorf("skipped[0] = %+v", skipped[0])
	}
}

func TestParseCaps(t *testing.T) {
	var answers []Answer
	for range MaxTxs + 2 {
		answers = append(answers, Answer{Kind: "expense", Amount: 1})
	}
	got, skipped := Parse(testIn(), answers)
	if len(got) != MaxTxs || len(skipped) != 2 {
		t.Errorf("got %d, skipped %d", len(got), len(skipped))
	}
}

func TestCheap(t *testing.T) {
	in := testIn()
	cases := []struct {
		text    string
		ok      bool
		kind    Kind
		amount  float64
		cat, cp string
	}{
		{"85", true, KindExpense, 85, "", ""},
		{"  ٨٥ ", true, KindExpense, 85, "", ""},
		{"food 85", true, KindExpense, 85, "cat-food", ""},
		{"Goods Shop 1,200.50", true, KindExpense, 1200.5, "", "cp-shop"},
		{"goods*12 10", true, KindExpense, 10, "", "cp-shop"},
		{"salary 40k", true, KindIncome, 40000, "cat-salary", ""},
		{"coffee 85", false, "", 0, "", ""},
		{"food 85 cash", false, "", 0, "", ""},
		{"lent Ahmed 500", false, "", 0, "", ""},
		{"0", false, "", 0, "", ""},
	}
	for _, c := range cases {
		g, ok := Cheap(in, c.text)
		if ok != c.ok {
			t.Errorf("%q: ok = %v", c.text, ok)
			continue
		}
		if ok && (g.Kind != c.kind || g.Amount != c.amount || g.CategoryID != c.cat || g.CounterpartyID != c.cp) {
			t.Errorf("%q = %+v", c.text, g)
		}
	}
}

func TestHashTags(t *testing.T) {
	text := "coffee 85 #work, uber 120 #Work #دبي a#b #trip-2026 ##x #"
	if got := HashTags(text); strings.Join(got, "|") != "work|دبي|trip-2026" {
		t.Errorf("HashTags = %q", got)
	}
	if got := StripHashTags("coffee 85 #work cash"); got != "coffee 85 cash" {
		t.Errorf("StripHashTags = %q", got)
	}
}

func TestParseTags(t *testing.T) {
	in := testIn()
	in.Text = "coffee 85 #work, uber 120"
	got, _ := Parse(in, []Answer{
		{Kind: "expense", Amount: 85, Tags: []string{"#WORK", "made-up"}, Note: "#work"},
		{Kind: "expense", Amount: 120},
	})
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "work" || got[0].Note != "" || len(got[1].Tags) != 0 {
		t.Errorf("tags = %+v", got)
	}
	// none placed: on every one
	got, _ = Parse(in, []Answer{{Kind: "expense", Amount: 85, Note: "#work"}, {Kind: "expense", Amount: 120}})
	if len(got[0].Tags) != 1 || len(got[1].Tags) != 1 {
		t.Errorf("unplaced tags = %+v", got)
	}
	g, ok := Cheap(in, "food 40 #work")
	if !ok || g.CategoryID != "cat-food" || len(g.Tags) != 1 || g.Tags[0] != "work" {
		t.Errorf("cheap = %+v %v", g, ok)
	}
}

func TestPromptDebtsAndBalances(t *testing.T) {
	in := testIn()
	in.Accounts[0].Balance = 1250.5
	in.Counterparties[1].Debts = []Debt{{Amount: -300, Currency: "EGP"}, {Amount: 20, Currency: "USD"}}
	in.Counterparties[0].Aliases = []string{"a1", "a2", "a3", "a4"}
	p := Prompt(in)
	for _, want := range []string{"A1: CIB, EGP, Bank account, 1250.5", "P2: Ahmed Ali, person; the user owes them 300 EGP; owes the user 20 USD", "| a1 | a2 | a3\n"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "a4") {
		t.Error("prompt has more than MaxAliases aliases")
	}
}

func TestFindCounterparty(t *testing.T) {
	cps := []Counterparty{
		{ID: "shop", Name: "Goods Shop", IsMerchant: true, Aliases: []string{"GOODS*12"}},
		{ID: "manuel", Name: "Manuel García"},
		{ID: "sara1", Name: "Sara Adel"},
		{ID: "sara2", Name: "Sara Nabil"},
		{ID: "uber", Name: "Uber", IsMerchant: true},
	}
	for _, c := range []struct {
		name     string
		merchant bool
		want     string
	}{
		{"goods shop.", true, "shop"},
		{"Goods*12", true, "shop"},
		{"manuel garcia", false, "manuel"},
		{"Manuel", false, "manuel"},
		{"Sara", false, ""},      // two Saras
		{"Uber Eats", true, ""}, // no prefix matching for businesses
		{"Manuel", true, ""},
		{"Nobody", false, ""},
	} {
		if got := FindCounterparty(c.name, c.merchant, cps); got != c.want {
			t.Errorf("FindCounterparty(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTooLong(t *testing.T) {
	if TooLong(strings.Repeat("ق", MaxTextLen)) {
		t.Error("MaxTextLen runes is too long")
	}
	if !TooLong(strings.Repeat("a", MaxTextLen+1)) {
		t.Error("over MaxTextLen isn't too long")
	}
}
