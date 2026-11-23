// Package visa checks money against a visa's financial requirements (the
// visa readiness tool): income, a job offer's salary, savings, a blocked
// account, an investment, plus yes/no conditions the user ticks.
package visa

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/omareloui/tyche/internal/payroll"
)

// Source tells where rules come from.
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceAI      Source = "ai"
	SourceManual  Source = "manual"
)

// Category is the kind of visa.
type Category string

const (
	CategoryNomad    Category = "nomad"
	CategoryPassive  Category = "passive"
	CategoryStudent  Category = "student"
	CategoryWork     Category = "work"
	CategoryInvestor Category = "investor"
	CategoryFamily   Category = "family"
	CategoryOther    Category = "other"
)

// Categories are every category, in the order shown.
var Categories = []Category{CategoryNomad, CategoryPassive, CategoryWork, CategoryStudent, CategoryInvestor, CategoryFamily, CategoryOther}

// Label names the category.
func (c Category) Label() string {
	switch c {
	case CategoryNomad:
		return "Digital nomad"
	case CategoryPassive:
		return "Passive income / retirement"
	case CategoryStudent:
		return "Student"
	case CategoryWork:
		return "Work"
	case CategoryInvestor:
		return "Investor"
	case CategoryFamily:
		return "Family"
	default:
		return "Other"
	}
}

// ParseCategory is the category named s, other when unknown.
func ParseCategory(s string) Category {
	c := Category(strings.ToLower(strings.TrimSpace(s)))
	if slices.Contains(Categories, c) {
		return c
	}
	return CategoryOther
}

// Kind is what a requirement asks.
type Kind string

const (
	// KindIncome is an income of any source (a month's or a year's).
	KindIncome Kind = "income"
	// KindSalary is a job offer's gross salary.
	KindSalary Kind = "salary"
	// KindSavings are funds held.
	KindSavings Kind = "savings"
	// KindBlocked are funds put in a blocked account.
	KindBlocked Kind = "blocked"
	// KindInvestment is money invested in the country.
	KindInvestment Kind = "investment"
	// KindCheck is a yes/no condition the user ticks.
	KindCheck Kind = "check"
)

// Kinds are every kind, in the order the form offers them.
var Kinds = []Kind{KindIncome, KindSalary, KindSavings, KindBlocked, KindInvestment, KindCheck}

// Label names the kind.
func (k Kind) Label() string {
	switch k {
	case KindIncome:
		return "Income"
	case KindSalary:
		return "Job offer's gross salary"
	case KindSavings:
		return "Savings"
	case KindBlocked:
		return "Blocked account"
	case KindInvestment:
		return "Investment"
	default:
		return "Condition"
	}
}

// Money tells the kind is an amount (not a condition).
func (k Kind) Money() bool { return k != KindCheck && k != "" }

// Funds tells the kind is money held (savings, blocked, investment), which
// a goal can save for.
func (k Kind) Funds() bool { return k == KindSavings || k == KindBlocked || k == KindInvestment }

// Period is what an amount is of.
type Period string

const (
	PeriodMonth Period = "month"
	PeriodYear  Period = "year"
	// PeriodOnce is a lump sum (funds held).
	PeriodOnce Period = "once"
)

// Option is a choice the built in rules depend on.
type Option = payroll.Option

// Choice is one of an option's values.
type Choice = payroll.Choice

// Requirement is one of a visa's conditions.
type Requirement struct {
	Kind   Kind    `json:"kind"`
	Period Period  `json:"period,omitempty"`
	Amount float64 `json:"amount,omitempty"`
	// PerDependent is added for each dependent, the last one repeating
	// (Spain's nomad visa: +75% of the base for the first, +25% each after).
	PerDependent []float64 `json:"per_dependent,omitempty"`
	// Group joins alternatives: one requirement of a group met is enough
	// (an income or savings).
	Group string `json:"group,omitempty"`
	// MonthsProof is how many of the last months' income has to reach the
	// amount (the bank statements asked).
	MonthsProof int `json:"months_proof,omitempty"`
	// Text explains it (a condition's text).
	Text string `json:"text,omitempty"`
}

// Need is the amount asked with the dependents.
func (q Requirement) Need(dependents int) float64 {
	need := q.Amount
	for i := 0; i < dependents && len(q.PerDependent) > 0; i++ {
		need += q.PerDependent[min(i, len(q.PerDependent)-1)]
	}
	return need
}

// Rule is a visa's requirements, in its currency.
type Rule struct {
	Country string `json:"country"`
	// Key is the visa's (a built in one's key, else its name normalized).
	Key      string   `json:"key,omitempty"`
	Name     string   `json:"name"`
	Category Category `json:"category"`
	Variant  string   `json:"variant,omitempty"`
	Year     int      `json:"year"`
	Currency string   `json:"currency"`
	Source   Source   `json:"source"`

	Requirements []Requirement `json:"requirements"`

	Notes     string    `json:"notes,omitempty"`
	Sources   []string  `json:"sources,omitempty"`
	FetchedAt time.Time `json:"fetched_at,omitzero"`
}

// Clone is a deep copy.
func (r *Rule) Clone() *Rule {
	c := *r
	c.Requirements = make([]Requirement, len(r.Requirements))
	for i, q := range r.Requirements {
		q.PerDependent = slices.Clone(q.PerDependent)
		c.Requirements[i] = q
	}
	c.Sources = slices.Clone(r.Sources)
	return &c
}

// MaxRequirements caps a rule's requirements.
const MaxRequirements = 30

// Validate tells what's wrong with the rule.
func (r *Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("the visa needs a name")
	}
	if len(r.Currency) != 3 {
		return errors.New("the currency has to be a 3 letters code")
	}
	if len(r.Requirements) == 0 {
		return errors.New("add at least one requirement")
	}
	if len(r.Requirements) > MaxRequirements {
		return fmt.Errorf("at most %d requirements", MaxRequirements)
	}
	for i, q := range r.Requirements {
		n := i + 1
		if !slices.Contains(Kinds, q.Kind) {
			return fmt.Errorf("requirement %d: unknown kind %q", n, q.Kind)
		}
		if !q.Kind.Money() {
			if strings.TrimSpace(q.Text) == "" {
				return fmt.Errorf("requirement %d: a condition needs its text", n)
			}
			continue
		}
		if q.Amount <= 0 || math.IsNaN(q.Amount) || math.IsInf(q.Amount, 0) {
			return fmt.Errorf("requirement %d: the amount has to be over 0", n)
		}
		switch q.Kind {
		case KindIncome, KindSalary:
			if q.Period != PeriodMonth && q.Period != PeriodYear {
				return fmt.Errorf("requirement %d: an income is a month's or a year's", n)
			}
		default:
			if q.Period != PeriodOnce {
				return fmt.Errorf("requirement %d: funds are a lump sum", n)
			}
		}
		for _, d := range q.PerDependent {
			if d < 0 || math.IsNaN(d) {
				return fmt.Errorf("requirement %d: amounts per dependent can't be negative", n)
			}
		}
		if q.MonthsProof < 0 || q.MonthsProof > 24 {
			return fmt.Errorf("requirement %d: months of proof are 0 to 24", n)
		}
		if q.MonthsProof > 0 && q.Kind != KindIncome {
			return fmt.Errorf("requirement %d: only an income has months of proof", n)
		}
	}
	return nil
}

// Input is the user's money, in the rule's currency.
type Input struct {
	// MonthlyIncomes are the last full months' incomes, oldest first.
	MonthlyIncomes []float64
	// Income is the income a month (their average, or typed).
	Income float64
	// IncomeTyped tells Income was typed: MonthlyIncomes aren't it.
	IncomeTyped bool
	// Offer is a job offer's gross salary a year, 0 when none.
	Offer float64
	// Savings are the funds held.
	Savings    float64
	Dependents int
	// Checked are the conditions ticked, by requirement index.
	Checked map[int]bool
}

// Status is how a requirement stands.
type Status string

const (
	StatusMet   Status = "met"
	StatusShort Status = "short"
	// StatusUnknown has no number to compare (no job offer typed).
	StatusUnknown Status = "unknown"
	// StatusTick is a condition not ticked yet.
	StatusTick Status = "tick"
)

// Item is a requirement checked.
type Item struct {
	Index int
	Requirement
	Need, Have, Gap float64
	Status          Status
	// MonthsMet is how many of the last MonthsProof months reached a
	// month's need (-1 when unknown: typed income, too few months).
	MonthsMet int
	// Months are the last MonthsProof months' incomes, oldest first.
	Months []float64
	// GroupMet tells another requirement of its group is met.
	GroupMet bool
}

// Met tells the item or its group is met.
func (it Item) Met() bool { return it.Status == StatusMet || it.GroupMet }

// Verdict is the whole check.
type Verdict string

const (
	VerdictReady Verdict = "ready"
	// VerdictTick has the money but conditions not ticked.
	VerdictTick Verdict = "tick"
	// VerdictUnknown lacks a number (the job offer).
	VerdictUnknown Verdict = "unknown"
	VerdictShort   Verdict = "short"
)

// Result is the rule checked against the input.
type Result struct {
	Items   []Item
	Verdict Verdict
	// FundsGap is what's missing in funds to meet the money requirements
	// (the smallest per unmet group), for a goal.
	FundsGap float64
	// Shortest is the smallest gap of the unmet money requirements, as a
	// share of its need, to rank visas.
	Shortest float64
}

// MonthlyNeed is an income requirement's need a month.
func (it Item) MonthlyNeed() float64 {
	if it.Period == PeriodYear {
		return it.Need / 12
	}
	return it.Need
}

// Check checks the rule against the input.
func Check(r *Rule, in Input) Result {
	res := Result{Items: make([]Item, len(r.Requirements))}
	for i, q := range r.Requirements {
		it := Item{Index: i, Requirement: q, MonthsMet: -1}
		if q.Kind == KindCheck {
			it.Status = StatusTick
			if in.Checked[i] {
				it.Status = StatusMet
			}
			res.Items[i] = it
			continue
		}
		it.Need = q.Need(in.Dependents)
		switch q.Kind {
		case KindIncome:
			it.Have = in.Income
			if q.Period == PeriodYear {
				it.Have = in.Income * 12
			}
		case KindSalary:
			it.Have = in.Offer
			if q.Period == PeriodMonth {
				it.Have = in.Offer / 12
			}
		default:
			it.Have = in.Savings
		}
		it.Have = math.Max(it.Have, 0)
		it.Gap = math.Max(it.Need-it.Have, 0)
		switch {
		case q.Kind == KindSalary && in.Offer <= 0:
			it.Status = StatusUnknown
		case it.Gap > 0.005:
			it.Status = StatusShort
		default:
			it.Status, it.Gap = StatusMet, 0
		}
		if q.Kind == KindIncome && q.MonthsProof > 0 && !in.IncomeTyped && len(in.MonthlyIncomes) >= q.MonthsProof {
			it.Months = in.MonthlyIncomes[len(in.MonthlyIncomes)-q.MonthsProof:]
			it.MonthsMet = 0
			for _, m := range it.Months {
				if m+0.005 >= it.MonthlyNeed() {
					it.MonthsMet++
				}
			}
			if it.MonthsMet < q.MonthsProof && it.Status == StatusMet {
				it.Status = StatusShort
			}
		}
		res.Items[i] = it
	}

	// Groups: one met is enough.
	met := map[string]bool{}
	for _, it := range res.Items {
		if it.Group != "" && it.Status == StatusMet {
			met[it.Group] = true
		}
	}
	for i := range res.Items {
		it := &res.Items[i]
		it.GroupMet = it.Group != "" && it.Status != StatusMet && met[it.Group]
	}

	// Verdict, funds gap per unmet group (or lone requirement).
	short, unknown, tick := false, false, false
	gaps := map[string]float64{}
	res.Shortest = math.Inf(1)
	for _, it := range res.Items {
		if it.Met() {
			continue
		}
		switch it.Status {
		case StatusShort:
			short = true
			if it.Need > 0 {
				res.Shortest = math.Min(res.Shortest, it.Gap/it.Need)
			}
		case StatusUnknown:
			unknown = true
		case StatusTick:
			tick = true
		}
		if it.Kind.Funds() && it.Status == StatusShort {
			g := it.Group
			if g == "" {
				g = fmt.Sprintf("#%d", it.Index)
			}
			if cur, ok := gaps[g]; !ok || it.Gap < cur {
				gaps[g] = it.Gap
			}
		}
	}
	// A group with an unmet income and no funds can't be saved for.
	for _, g := range gaps {
		res.FundsGap += g
	}
	if math.IsInf(res.Shortest, 1) {
		res.Shortest = 0
	}
	switch {
	case short:
		res.Verdict = VerdictShort
	case unknown:
		res.Verdict = VerdictUnknown
	case tick:
		res.Verdict = VerdictTick
	default:
		res.Verdict = VerdictReady
	}
	return res
}

// MoneyItems are the money requirements.
func (res Result) MoneyItems() []Item {
	var out []Item
	for _, it := range res.Items {
		if it.Kind.Money() {
			out = append(out, it)
		}
	}
	return out
}

// Checks are the conditions.
func (res Result) Checks() []Item {
	var out []Item
	for _, it := range res.Items {
		if !it.Kind.Money() {
			out = append(out, it)
		}
	}
	return out
}

// KeyOf is a typed visa name's key: lowercased, runs of anything but
// letters and digits as one dash.
func KeyOf(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}
