// Package payroll works a salary's take-home pay out of a country's rules:
// income tax brackets, allowances and deductions, and the employee's social
// contributions. The rules are built in for a few countries (Builtin), else
// estimated by the AI or typed by the user, all in the same Rules.
package payroll

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Source tells where rules come from.
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceAI      Source = "ai"
	SourceManual  Source = "manual"
)

// Bracket is a band of the tax scale: Rate on the income up to UpTo (from
// the bracket before's UpTo), 0 for the last, open ended, one. Amounts are
// a year's.
type Bracket struct {
	UpTo float64 `json:"up_to"`
	Rate float64 `json:"rate"`
}

// Table is a tax scale used for taxable incomes over Above (Egypt takes the
// lower brackets away from the higher incomes).
type Table struct {
	Above    float64   `json:"above"`
	Brackets []Bracket `json:"brackets"`
}

// Allowance is the tax free income everyone gets, deducted from the
// taxable income, shrunk by TaperRate per unit of income over TaperFrom
// (UK's £1 per £2 over £100,000); AsCredit taxes it at the scale and takes
// that off the tax instead (Spain's personal minimum).
type Allowance struct {
	Amount    float64 `json:"amount"`
	TaperFrom float64 `json:"taper_from,omitempty"`
	TaperRate float64 `json:"taper_rate,omitempty"`
	AsCredit  bool    `json:"as_credit,omitempty"`
}

// Deduction is a fixed amount a year off the taxable income (a standard
// deduction, an exemption).
type Deduction struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

// Step is one piece of a reduction depending on the income: Amount less
// Slope per unit of income over From, for incomes in [From, UpTo) (UpTo 0 =
// no end).
type Step struct {
	From   float64 `json:"from"`
	UpTo   float64 `json:"up_to"`
	Amount float64 `json:"amount"`
	Slope  float64 `json:"slope,omitempty"`
}

// Contribution is one of the employee's social contributions (pension,
// health, unemployment...): Rate of the pay between Floor and Cap (per
// month when Monthly, else a year), or by Bands of the pay when set (UK's
// National Insurance). Deductible ones come off the taxable income.
type Contribution struct {
	Name       string    `json:"name"`
	Rate       float64   `json:"rate"`
	Floor      float64   `json:"floor,omitempty"`
	Cap        float64   `json:"cap,omitempty"`
	Monthly    bool      `json:"monthly,omitempty"`
	Deductible bool      `json:"deductible,omitempty"`
	Bands      []Bracket `json:"bands,omitempty"`
	// Credit gives a tax credit of the contribution, at each scale's credit
	// rate (Canada's CPP and EI).
	Credit bool `json:"credit,omitempty"`
}

// Credit is a non-refundable tax credit: Amount at Rate (the scale's lowest
// rate when 0) off the tax, shrunk linearly to Floor for net incomes
// between TaperFrom and TaperTo (Canada's basic personal amount).
type Credit struct {
	Name      string  `json:"name"`
	Amount    float64 `json:"amount"`
	Rate      float64 `json:"rate,omitempty"`
	TaperFrom float64 `json:"taper_from,omitempty"`
	TaperTo   float64 `json:"taper_to,omitempty"`
	Floor     float64 `json:"floor,omitempty"`
}

// Layer is a second income tax on the same taxable income, with its own
// scale and credits (a province's, a state's): Surtax bands are on its tax
// after the credits (Ontario's surtax).
type Layer struct {
	Name     string    `json:"name"`
	Brackets []Bracket `json:"brackets"`
	Credits  []Credit  `json:"credits,omitempty"`
	Surtax   []Bracket `json:"surtax,omitempty"`
}

// Option is a choice the rules depend on (filing status, nationality,
// region), for the built in rules.
type Option struct {
	Key     string
	Label   string
	Choices []Choice
}

// Choice is one of an option's values.
type Choice struct{ Value, Label string }

// Rules are a country's (or region's) rules of a tax year, in its currency,
// amounts a year's.
type Rules struct {
	Country  string `json:"country"`
	Region   string `json:"region,omitempty"`
	Variant  string `json:"variant,omitempty"`
	Year     int    `json:"year"`
	Currency string `json:"currency"`
	Source   Source `json:"source"`

	Brackets      []Bracket      `json:"brackets"`
	Tables        []Table        `json:"tables,omitempty"`
	Allowance     Allowance      `json:"allowance"`
	Deductions    []Deduction    `json:"deductions,omitempty"`
	WorkReduction []Step         `json:"work_reduction,omitempty"`
	Contributions []Contribution `json:"contributions,omitempty"`
	// Credits are the main scale's tax credits, TaxName its name when there
	// are Layers (e.g. Federal).
	Credits []Credit `json:"credits,omitempty"`
	TaxName string   `json:"tax_name,omitempty"`
	Layers  []Layer  `json:"layers,omitempty"`
	// PaysPerYear is how many pays the year's salary is split in (Spain's
	// 14), 12 when 0.
	PaysPerYear int `json:"pays_per_year,omitempty"`

	Notes     string    `json:"notes,omitempty"`
	Sources   []string  `json:"sources,omitempty"`
	FetchedAt time.Time `json:"fetched_at,omitzero"`
}

// Pays is how many pays a year, 12 unless set.
func (r *Rules) Pays() int {
	if r.PaysPerYear > 0 {
		return r.PaysPerYear
	}
	return 12
}

// NoIncomeTax tells the rules tax nothing.
func (r *Rules) NoIncomeTax() bool {
	if len(r.Layers) > 0 {
		return false
	}
	for _, b := range r.Brackets {
		if b.Rate > 0 {
			return false
		}
	}
	for _, t := range r.Tables {
		for _, b := range t.Brackets {
			if b.Rate > 0 {
				return false
			}
		}
	}
	return true
}

// Limits of what Validate takes.
const (
	MaxBrackets      = 20
	MaxTables        = 10
	MaxContributions = 12
	MaxDeductions    = 10
	MaxSteps         = 10
	maxName          = 80
)

// ErrInvalid wraps every problem Validate finds.
var ErrInvalid = errors.New("invalid rules")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func validRate(r float64) bool { return r >= 0 && r <= 1 && !math.IsNaN(r) }

func validAmount(a float64) bool { return a >= 0 && !math.IsNaN(a) && !math.IsInf(a, 0) }

func validateBrackets(what string, bs []Bracket, needOne bool) error {
	if needOne && len(bs) == 0 {
		return invalid("%s: no brackets", what)
	}
	if len(bs) > MaxBrackets {
		return invalid("%s: more than %d brackets", what, MaxBrackets)
	}
	prev := 0.0
	for i, b := range bs {
		if !validRate(b.Rate) {
			return invalid("%s: rate %d out of 0–100%%", what, i+1)
		}
		if !validAmount(b.UpTo) {
			return invalid("%s: bracket %d's limit", what, i+1)
		}
		last := i == len(bs)-1
		if b.UpTo == 0 && !last {
			return invalid("%s: only the last bracket can be open ended", what)
		}
		if b.UpTo != 0 && b.UpTo <= prev {
			return invalid("%s: brackets must go up", what)
		}
		prev = b.UpTo
	}
	return nil
}

// Validate checks the rules make sense: rates within 0–100%, brackets going
// up, caps over floors.
func (r *Rules) Validate() error {
	if len(r.Currency) != 3 {
		return invalid("currency")
	}
	if r.PaysPerYear < 0 || r.PaysPerYear > 24 {
		return invalid("pays a year")
	}
	if err := validateBrackets("tax", r.Brackets, true); err != nil {
		return err
	}
	if len(r.Tables) > MaxTables {
		return invalid("more than %d tables", MaxTables)
	}
	for i, t := range r.Tables {
		if !validAmount(t.Above) || t.Above == 0 {
			return invalid("table %d's threshold", i+1)
		}
		if err := validateBrackets(fmt.Sprintf("table %d", i+1), t.Brackets, true); err != nil {
			return err
		}
	}
	a := r.Allowance
	if !validAmount(a.Amount) || !validAmount(a.TaperFrom) || !validRate(a.TaperRate) {
		return invalid("allowance")
	}
	if len(r.Deductions) > MaxDeductions {
		return invalid("more than %d deductions", MaxDeductions)
	}
	for _, d := range r.Deductions {
		if !validAmount(d.Amount) || len(d.Name) > maxName {
			return invalid("deduction %q", d.Name)
		}
	}
	if len(r.WorkReduction) > MaxSteps {
		return invalid("more than %d reduction steps", MaxSteps)
	}
	for i, s := range r.WorkReduction {
		if !validAmount(s.From) || !validAmount(s.UpTo) || !validAmount(s.Amount) || !validAmount(s.Slope) || (s.UpTo != 0 && s.UpTo <= s.From) {
			return invalid("reduction step %d", i+1)
		}
	}
	if len(r.Contributions) > MaxContributions {
		return invalid("more than %d contributions", MaxContributions)
	}
	for _, c := range r.Contributions {
		if strings.TrimSpace(c.Name) == "" || len(c.Name) > maxName {
			return invalid("a contribution's name")
		}
		if !validRate(c.Rate) || !validAmount(c.Floor) || !validAmount(c.Cap) {
			return invalid("contribution %q", c.Name)
		}
		if c.Cap != 0 && c.Cap < c.Floor {
			return invalid("contribution %q: cap under its floor", c.Name)
		}
		if err := validateBrackets("contribution "+c.Name, c.Bands, false); err != nil {
			return err
		}
	}
	if err := validateCredits(r.Credits); err != nil {
		return err
	}
	if len(r.Layers) > MaxTables {
		return invalid("more than %d layers", MaxTables)
	}
	for _, l := range r.Layers {
		if strings.TrimSpace(l.Name) == "" || len(l.Name) > maxName {
			return invalid("a layer's name")
		}
		if err := validateBrackets(l.Name, l.Brackets, true); err != nil {
			return err
		}
		if err := validateBrackets(l.Name+" surtax", l.Surtax, false); err != nil {
			return err
		}
		if err := validateCredits(l.Credits); err != nil {
			return err
		}
	}
	return nil
}

func validateCredits(cs []Credit) error {
	if len(cs) > MaxDeductions {
		return invalid("more than %d credits", MaxDeductions)
	}
	for _, c := range cs {
		if strings.TrimSpace(c.Name) == "" || len(c.Name) > maxName || !validAmount(c.Amount) || !validRate(c.Rate) ||
			!validAmount(c.TaperFrom) || !validAmount(c.TaperTo) || !validAmount(c.Floor) || (c.TaperTo != 0 && c.TaperTo < c.TaperFrom) {
			return invalid("credit %q", c.Name)
		}
	}
	return nil
}

// Input is the salary to work out, a year's (bonus included apart).
type Input struct {
	AnnualGross float64
	// Bonus is a year's bonus on top, taxed with the year's income;
	// BonusRate one as a share of AnnualGross instead (used when set).
	Bonus     float64
	BonusRate float64
	// OtherRate is a flat tax on the gross the rules don't have (a US
	// state's), 0–1.
	OtherRate float64
	// Pays is how many pays a year, the rules' when 0.
	Pays int
}

// Slice is what a tax bracket takes.
type Slice struct {
	From, To float64 // To 0 = open ended
	Rate     float64
	Taxed    float64 // the income in it
	Tax      float64
}

// Line is an amount taken off the pay.
type Line struct {
	Name       string
	Rate       float64 // 0 when banded
	Amount     float64
	Deductible bool
}

// Result is what a salary comes to, a year's amounts.
type Result struct {
	Gross   float64
	Bonus   float64
	Taxable float64
	// Allowance and Deductions are what came off the taxable income
	// (Reduction the work income's).
	Allowance  float64
	Deductions []Deduction
	Reduction  float64
	Tax        float64
	Credit     float64 // the allowance's (AsCredit) and Credits'
	// TaxName names the main scale's tax when there are Layers, each
	// worked out in LayerResults.
	TaxName string
	Layers  []LayerResult
	Slices     []Slice
	Contribs   []Line
	Other      float64 // Input.OtherRate's tax

	Net           float64
	Pays          int
	EffectiveRate float64
	MarginalRate  float64
	// BonusNet is what the bonus adds to the take home pay.
	BonusNet float64
}

// LayerResult is what a Layer takes.
type LayerResult struct {
	Name   string
	Slices []Slice
	Credit float64
	Surtax float64
	// Tax is the layer's, after its credits and with its surtax.
	Tax float64
}

// MainTax is the main scale's tax, the layers' left out.
func (res *Result) MainTax() float64 {
	t := res.Tax
	for _, l := range res.Layers {
		t -= l.Tax
	}
	return t
}

// TotalContribs sums the contributions.
func (res *Result) TotalContribs() float64 {
	t := 0.0
	for _, c := range res.Contribs {
		t += c.Amount
	}
	return t
}

// NetMonthly is the take home pay a month, the bonus spread over the year.
func (res *Result) NetMonthly() float64 { return res.Net / 12 }

// NetPerPay is the take home pay of each pay, the bonus left out.
func (res *Result) NetPerPay() float64 {
	if res.Pays <= 0 {
		return 0
	}
	return (res.Net - res.BonusNet) / float64(res.Pays)
}

// table is the scale taxable is taxed at.
func (r *Rules) table(taxable float64) []Bracket {
	bs, above := r.Brackets, 0.0
	for _, t := range r.Tables {
		if taxable > t.Above && t.Above >= above {
			bs, above = t.Brackets, t.Above
		}
	}
	return bs
}

func scale(bs []Bracket, income float64) (tax float64, slices []Slice) {
	from := 0.0
	for _, b := range bs {
		if income <= from {
			break
		}
		to := b.UpTo
		in := income - from
		if to != 0 {
			in = math.Min(income, to) - from
		}
		t := in * b.Rate
		tax += t
		slices = append(slices, Slice{From: from, To: to, Rate: b.Rate, Taxed: in, Tax: t})
		if to == 0 {
			break
		}
		from = to
	}
	return tax, slices
}

func (c Contribution) amount(gross float64) float64 {
	base := gross
	floor, cap := c.Floor, c.Cap
	if c.Monthly {
		floor, cap = floor*12, cap*12
	}
	if cap > 0 {
		base = math.Min(base, cap)
	}
	if base < floor {
		base = floor
	}
	if gross <= 0 {
		return 0
	}
	if len(c.Bands) > 0 {
		t, _ := scale(c.Bands, base)
		return t
	}
	return base * c.Rate
}

// lowestRate is the scale's first rate over 0, what its credits are worth.
func lowestRate(bs []Bracket) float64 {
	for _, b := range bs {
		if b.Rate > 0 {
			return b.Rate
		}
	}
	return 0
}

// creditsOf is what the credits and the credited contributions take off a
// scale's tax, for the net income.
func creditsOf(cs []Credit, contribs float64, bs []Bracket, income float64) float64 {
	low := lowestRate(bs)
	total := contribs * low
	for _, c := range cs {
		amount := c.Amount
		if c.TaperTo > c.TaperFrom && income > c.TaperFrom {
			share := math.Min((income-c.TaperFrom)/(c.TaperTo-c.TaperFrom), 1)
			amount -= (c.Amount - c.Floor) * share
		}
		rate := c.Rate
		if rate == 0 {
			rate = low
		}
		total += math.Max(amount, 0) * rate
	}
	return total
}

func reduction(steps []Step, income float64) float64 {
	for _, s := range steps {
		if income >= s.From && (s.UpTo == 0 || income < s.UpTo) {
			return math.Max(s.Amount-s.Slope*(income-s.From), 0)
		}
	}
	return 0
}

// compute works the take home pay of a year's gross out.
func (r *Rules) compute(gross, otherRate float64) Result {
	res := Result{Gross: gross}
	deductible := 0.0
	for _, c := range r.Contributions {
		a := c.amount(gross)
		res.Contribs = append(res.Contribs, Line{Name: c.Name, Rate: c.Rate, Amount: a, Deductible: c.Deductible})
		if c.Deductible {
			deductible += a
		}
	}

	income := math.Max(gross-deductible, 0)
	res.Reduction = math.Min(reduction(r.WorkReduction, income), income)
	taxable := income - res.Reduction
	for _, d := range r.Deductions {
		a := math.Min(d.Amount, taxable)
		taxable -= a
		res.Deductions = append(res.Deductions, Deduction{Name: d.Name, Amount: a})
	}

	allow := r.Allowance.Amount
	if r.Allowance.TaperRate > 0 && gross > r.Allowance.TaperFrom {
		allow = math.Max(allow-(gross-r.Allowance.TaperFrom)*r.Allowance.TaperRate, 0)
	}
	if !r.Allowance.AsCredit {
		allow = math.Min(allow, taxable)
		taxable -= allow
	}
	res.Allowance = allow
	res.Taxable = taxable

	contribCredit := 0.0
	for i, c := range r.Contributions {
		if c.Credit {
			contribCredit += res.Contribs[i].Amount
		}
	}

	main := r.table(taxable)
	res.Tax, res.Slices = scale(main, taxable)
	credit := creditsOf(r.Credits, contribCredit, main, income)
	if r.Allowance.AsCredit && allow > 0 {
		c, _ := scale(main, math.Min(allow, taxable))
		credit += c
	}
	res.Credit = math.Min(credit, res.Tax)
	res.Tax -= res.Credit
	res.TaxName = r.TaxName

	for _, l := range r.Layers {
		lr := LayerResult{Name: l.Name}
		var basic float64
		basic, lr.Slices = scale(l.Brackets, taxable)
		lr.Credit = math.Min(creditsOf(l.Credits, contribCredit, l.Brackets, income), basic)
		basic -= lr.Credit
		lr.Surtax, _ = scale(l.Surtax, basic)
		lr.Tax = basic + lr.Surtax
		res.Tax += lr.Tax
		res.Layers = append(res.Layers, lr)
	}
	res.Other = gross * otherRate
	res.Net = gross - res.Tax - res.TotalContribs() - res.Other
	return res
}

// Compute works out what the salary comes to under the rules.
func (r *Rules) Compute(in Input) Result {
	gross := math.Max(in.AnnualGross, 0)
	bonus := math.Max(in.Bonus, 0)
	if in.BonusRate > 0 {
		bonus = gross * in.BonusRate
	}
	res := r.compute(gross+bonus, in.OtherRate)
	res.Bonus = bonus
	if bonus > 0 {
		res.BonusNet = res.Net - r.compute(gross, in.OtherRate).Net
	}
	res.Pays = in.Pays
	if res.Pays <= 0 {
		res.Pays = r.Pays()
	}
	if res.Gross > 0 {
		res.EffectiveRate = 1 - res.Net/res.Gross
	}
	// The marginal rate: what's kept of a bit more pay.
	d := math.Max(res.Gross*0.001, 1)
	more := r.compute(res.Gross+d, in.OtherRate)
	res.MarginalRate = math.Max(1-(more.Net-res.Net)/d, 0)
	return res
}

// GrossFor is the year's gross (bonus aside) that takes home net a year
// (the bonus' take-home left out), false when no gross does (a net over
// what the rules could ever leave).
func (r *Rules) GrossFor(net float64, in Input) (float64, bool) {
	if net <= 0 {
		return 0, true
	}
	netOf := func(g float64) float64 {
		in.AnnualGross = g
		res := r.Compute(in)
		return res.Net - res.BonusNet
	}
	lo, hi := 0.0, math.Max(net, 1)
	for netOf(hi) < net {
		lo = hi
		hi *= 2
		if hi > 1e13 {
			return 0, false
		}
	}
	for range 200 {
		mid := (lo + hi) / 2
		if netOf(mid) < net {
			lo = mid
		} else {
			hi = mid
		}
		if hi-lo < 0.005 {
			break
		}
	}
	return math.Round(hi*100) / 100, true
}

// Clone is a deep copy, to change without touching the original.
func (r *Rules) Clone() *Rules {
	c := *r
	c.Brackets = slices.Clone(r.Brackets)
	c.Tables = make([]Table, len(r.Tables))
	for i, t := range r.Tables {
		c.Tables[i] = Table{Above: t.Above, Brackets: slices.Clone(t.Brackets)}
	}
	c.Deductions = slices.Clone(r.Deductions)
	c.WorkReduction = slices.Clone(r.WorkReduction)
	c.Contributions = make([]Contribution, len(r.Contributions))
	for i, ct := range r.Contributions {
		ct.Bands = slices.Clone(ct.Bands)
		c.Contributions[i] = ct
	}
	c.Sources = slices.Clone(r.Sources)
	c.Credits = slices.Clone(r.Credits)
	c.Layers = make([]Layer, len(r.Layers))
	for i, l := range r.Layers {
		c.Layers[i] = Layer{Name: l.Name, Brackets: slices.Clone(l.Brackets), Credits: slices.Clone(l.Credits), Surtax: slices.Clone(l.Surtax)}
	}
	return &c
}
