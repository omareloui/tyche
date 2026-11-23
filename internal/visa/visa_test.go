package visa

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestNeedDependents(t *testing.T) {
	q := Requirement{Kind: KindIncome, Period: PeriodMonth, Amount: 100, PerDependent: []float64{75, 25}}
	for deps, want := range map[int]float64{0: 100, 1: 175, 2: 200, 4: 250} {
		if got := q.Need(deps); !near(got, want) {
			t.Errorf("Need(%d) = %v, want %v", deps, got, want)
		}
	}
}

func TestCheckGroupsAndFunds(t *testing.T) {
	r := &Rule{Name: "x", Currency: "EUR", Requirements: []Requirement{
		{Kind: KindIncome, Period: PeriodYear, Amount: 1200, Group: "means"},
		{Kind: KindSavings, Period: PeriodOnce, Amount: 1000, Group: "means"},
		{Kind: KindCheck, Text: "insured"},
	}}
	res := Check(r, Input{Income: 50, Savings: 400})
	if res.Verdict != VerdictShort || !near(res.FundsGap, 600) {
		t.Fatalf("short: %+v", res)
	}
	res = Check(r, Input{Income: 100, Savings: 0})
	if res.Verdict != VerdictTick || !res.Items[1].GroupMet || res.FundsGap != 0 {
		t.Fatalf("income meets the group: %+v", res)
	}
	res = Check(r, Input{Income: 100, Checked: map[int]bool{2: true}})
	if res.Verdict != VerdictReady {
		t.Fatalf("ready: %+v", res.Verdict)
	}
}

func TestCheckMonthsProofAndOffer(t *testing.T) {
	r := &Rule{Name: "x", Currency: "EUR", Requirements: []Requirement{
		{Kind: KindIncome, Period: PeriodMonth, Amount: 100, MonthsProof: 3},
		{Kind: KindSalary, Period: PeriodMonth, Amount: 50},
	}}
	res := Check(r, Input{MonthlyIncomes: []float64{0, 200, 90, 200}, Income: 150})
	if it := res.Items[0]; it.MonthsMet != 2 || it.Status != StatusShort {
		t.Fatalf("months: %+v", it)
	}
	if res.Items[1].Status != StatusUnknown {
		t.Fatalf("no offer: %+v", res.Items[1])
	}
	res = Check(r, Input{MonthlyIncomes: []float64{0, 200, 90, 200}, Income: 150, IncomeTyped: true, Offer: 1200})
	if res.Items[0].Status != StatusMet || res.Items[0].MonthsMet != -1 || res.Verdict != VerdictReady {
		t.Fatalf("typed income / offer: %+v", res)
	}
}

func TestBuiltinsValidate(t *testing.T) {
	for _, c := range BuiltinCountries() {
		for _, v := range BuiltinVisas(c) {
			opts := BuiltinOptions(c, v.Key)
			combos := []map[string]string{{}}
			for _, o := range opts {
				for _, ch := range o.Choices {
					combos = append(combos, map[string]string{o.Key: ch.Value})
				}
			}
			for _, o := range combos {
				r, ok := Builtin(c, v.Key, o)
				if !ok {
					t.Fatalf("%s/%s missing", c, v.Key)
				}
				if err := r.Validate(); err != nil {
					t.Errorf("%s/%s %v: %v", c, v.Key, o, err)
				}
			}
		}
	}
	if r, _ := Builtin("es", "digital-nomad", nil); !near(r.Requirements[0].Amount, 2849) {
		t.Errorf("ES DNV = %v", r.Requirements[0].Amount)
	}
	if r, _ := Builtin("US", "eb-5", map[string]string{"area": "tea"}); !near(r.Requirements[0].Amount, 800_000) {
		t.Errorf("EB-5 TEA = %v", r.Requirements[0].Amount)
	}
	if r, _ := Builtin("NZ", "skilled-migrant", nil); !near(r.Requirements[0].Amount, 72_800) {
		t.Errorf("NZ median a year = %v", r.Requirements[0].Amount)
	}
	if r, _ := Builtin("EE", "employment", map[string]string{"level": "top"}); !near(r.Requirements[0].Amount, 3138) {
		t.Errorf("EE top specialist = %v", r.Requirements[0].Amount)
	}
	if r, _ := Builtin("CA", "express-entry", nil); !near(r.Requirements[0].Need(3), 28362) {
		t.Errorf("CA funds of 4 = %v", r.Requirements[0].Need(3))
	}
}

func TestKeyOf(t *testing.T) {
	if got := KeyOf("  Digital Nomad (D8) visa "); got != "digital-nomad-d8-visa" {
		t.Errorf("KeyOf = %q", got)
	}
}
