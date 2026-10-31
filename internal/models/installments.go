package models

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
)

// InstallmentStrategy is how a plan's interest is worked out.
type InstallmentStrategy uint8

const (
	// StrategyKnownAmount is a monthly amount given as-is (e.g. a shop's or
	// a BNPL app's quote), or the price split evenly with no interest. What
	// it costs over the price counts as interest.
	StrategyKnownAmount InstallmentStrategy = iota
	// StrategyFlat charges the rate on the whole financed amount for the
	// whole term, so equal payments (most personal and car loans quoted as
	// "flat rate", and credit card installment plans).
	StrategyFlat
	// StrategyReducing charges the rate on the remaining balance with equal
	// payments, less interest and more principal every month (an amortized
	// loan or mortgage).
	StrategyReducing
	// StrategyDecreasing charges the rate on the remaining balance with the
	// same principal every month, so the payments go down over time (common
	// for car loans).
	StrategyDecreasing
)

func InstallmentStrategies() []InstallmentStrategy {
	return []InstallmentStrategy{StrategyKnownAmount, StrategyFlat, StrategyReducing, StrategyDecreasing}
}

func (s InstallmentStrategy) String() string {
	return [...]string{"known", "flat", "reducing", "decreasing"}[s]
}

func (s InstallmentStrategy) Label() string {
	return [...]string{"Known amount", "Flat rate", "Reducing, equal payments", "Reducing, decreasing payments"}[s]
}

func (s InstallmentStrategy) Description() string {
	return [...]string{
		"You know the monthly amount, or it's interest-free",
		"Interest on the full amount for the whole term",
		"Interest on what's left, the same payment every month",
		"Interest on what's left, payments go down every month",
	}[s]
}

func (s InstallmentStrategy) HasRate() bool {
	return s != StrategyKnownAmount
}

func InstallmentStrategyFromString(str string) InstallmentStrategy {
	for _, s := range InstallmentStrategies() {
		if s.String() == str {
			return s
		}
	}
	return StrategyKnownAmount
}

// RatePeriod is what an interest rate is quoted per.
type RatePeriod uint8

const (
	RatePerYear RatePeriod = iota
	RatePerMonth
)

func (p RatePeriod) String() string {
	return [...]string{"year", "month"}[p]
}

// InstallmentPlan is how a purchase is paid over months: the strategy, its
// rate and the extras banks and shops add around it. Percentages are 0-100.
type InstallmentPlan struct {
	Strategy InstallmentStrategy `json:"strategy"`
	// Months is the number of monthly payments. A known amount without it
	// pays the price off in as many months as needed.
	Months int `json:"months"`
	// Amount is the monthly payment of a known amount plan, the price split
	// evenly over the months when 0.
	Amount     float64    `json:"amount,omitempty"`
	Rate       float64    `json:"rate,omitempty"`
	RatePeriod RatePeriod `json:"rate_period,omitempty"`

	// DownPayment is paid on the day, an amount or a percentage of the price.
	DownPayment    float64 `json:"down_payment,omitempty"`
	DownPaymentPct bool    `json:"down_payment_pct,omitempty"`
	// AdminFee is a one-time fee, an amount or a percentage of the financed
	// amount, paid on the day or added to the loan.
	AdminFee         float64 `json:"admin_fee,omitempty"`
	AdminFeePct      bool    `json:"admin_fee_pct,omitempty"`
	AdminFeeFinanced bool    `json:"admin_fee_financed,omitempty"`
	// MonthlyFee is added to every payment.
	MonthlyFee float64 `json:"monthly_fee,omitempty"`
	// InsuranceRate is a yearly percentage of the financed amount, paid with
	// every payment (e.g. life or car insurance on a loan).
	InsuranceRate float64 `json:"insurance_rate,omitempty"`
	// BalloonPct is a percentage of the price left to the last payment, the
	// others being lower (e.g. car financing with a residual value).
	BalloonPct float64 `json:"balloon_pct,omitempty"`
	// GraceMonths go by before the first payment. Interest still runs on
	// them: added to the balance of reducing plans, to the term of flat ones.
	GraceMonths int `json:"grace_months,omitempty"`
}

// InstallmentRow is one payment of a plan.
type InstallmentRow struct {
	// Month counts from the purchase, grace months included.
	Month                                    int
	Payment, Principal, Interest, Fees, Left float64
}

// InstallmentSchedule is a plan worked out for a price.
type InstallmentSchedule struct {
	Plan  InstallmentPlan
	Price float64
	// DownPayment and UpfrontFees are paid on the day.
	DownPayment, UpfrontFees float64
	// Financed is what's borrowed, a financed admin fee included.
	Financed float64
	Rows     []InstallmentRow
	// TotalPaid is everything paid, on the day included.
	TotalPaid, TotalInterest, TotalFees float64
	// APR is the effective yearly rate of the whole plan, fees included, to
	// compare plans of different strategies. 0 when it costs nothing extra.
	APR float64
}

var (
	ErrInstallmentMonths      = errors.New("the plan needs its months")
	ErrInstallmentKnownAmount = errors.New("the plan needs its months or its monthly amount")
	ErrInstallmentDownPayment = errors.New("the down payment and the last payment are more than the price")
	ErrInstallmentTooLow      = errors.New("the monthly amount doesn't pay off the price")
)

// Schedule works the plan out for a price.
func (p InstallmentPlan) Schedule(price float64) (*InstallmentSchedule, error) {
	s := &InstallmentSchedule{Plan: p, Price: price}

	s.DownPayment = p.DownPayment
	if p.DownPaymentPct {
		s.DownPayment = price * p.DownPayment / 100
	}
	balloon := price * p.BalloonPct / 100
	borrowed := price - s.DownPayment
	if borrowed < 0 || borrowed < balloon {
		return nil, ErrInstallmentDownPayment
	}

	adminFee := p.AdminFee
	if p.AdminFeePct {
		adminFee = borrowed * p.AdminFee / 100
	}
	s.Financed = borrowed
	if p.AdminFeeFinanced {
		s.Financed += adminFee
	} else {
		s.UpfrontFees = adminFee
	}

	n := p.Months
	if p.Strategy == StrategyKnownAmount && n <= 0 {
		if p.Amount <= 0 {
			return nil, ErrInstallmentKnownAmount
		}
		// As many months as the amount takes to pay it off, split evenly.
		n = int(math.Ceil(math.Floor((s.Financed-balloon)/p.Amount*100) / 100))
		p.Amount = (s.Financed - balloon) / float64(max(n, 1))
		s.Plan.Months, s.Plan.Amount = n, p.Amount
	}
	if n <= 0 {
		return nil, ErrInstallmentMonths
	}

	r := p.monthlyRate()
	fees := p.MonthlyFee + s.Financed*p.InsuranceRate/100/12
	g := p.GraceMonths

	// Reducing plans add the grace months' interest to the balance.
	left := s.Financed
	if p.Strategy == StrategyReducing || p.Strategy == StrategyDecreasing {
		left *= math.Pow(1+r, float64(g))
	}

	var pmt, principal, flatInterest float64
	switch p.Strategy {
	case StrategyKnownAmount:
		pmt = p.Amount
		if pmt <= 0 {
			pmt = (s.Financed - balloon) / float64(n)
		}
		// What's paid over the financed amount is spread as interest.
		principal = (s.Financed - balloon) / float64(n)
		flatInterest = pmt - principal
		if flatInterest < -0.005 {
			return nil, ErrInstallmentTooLow
		}
		flatInterest = max(flatInterest, 0)
	case StrategyFlat:
		principal = (s.Financed - balloon) / float64(n)
		flatInterest = s.Financed * r * float64(n+g) / float64(n)
		pmt = principal + flatInterest
	case StrategyReducing:
		if r == 0 {
			pmt = (left - balloon) / float64(n)
		} else {
			pmt = (left - balloon/math.Pow(1+r, float64(n))) * r / (1 - math.Pow(1+r, -float64(n)))
		}
	case StrategyDecreasing:
		principal = (left - balloon) / float64(n)
	}

	for i := 1; i <= n; i++ {
		row := InstallmentRow{Month: g + i, Fees: fees}
		switch p.Strategy {
		case StrategyKnownAmount, StrategyFlat:
			row.Principal, row.Interest = principal, flatInterest
		case StrategyReducing:
			row.Interest = left * r
			row.Principal = pmt - row.Interest
		case StrategyDecreasing:
			row.Interest = left * r
			row.Principal = principal
		}
		if i == n {
			row.Principal += balloon
		}
		left -= row.Principal
		row.Left = math.Max(0, left)
		row.Payment = row.Principal + row.Interest + row.Fees

		s.TotalInterest += row.Interest
		s.TotalFees += row.Fees
		s.TotalPaid += row.Payment
		s.Rows = append(s.Rows, row)
	}

	s.TotalFees += s.UpfrontFees
	s.TotalPaid += s.DownPayment + s.UpfrontFees
	s.APR = s.apr()
	return s, nil
}

func (p InstallmentPlan) monthlyRate() float64 {
	if !p.Strategy.HasRate() {
		return 0
	}
	if p.RatePeriod == RatePerMonth {
		return p.Rate / 100
	}
	return p.Rate / 100 / 12
}

// apr is the effective yearly rate making the payments worth what's
// borrowed on the day: the price less the down payment and the fees paid on
// the day.
func (s *InstallmentSchedule) apr() float64 {
	loan := s.Price - s.DownPayment - s.UpfrontFees
	if loan <= 0 || s.TotalPaid <= s.Price+0.005 {
		return 0
	}

	worth := func(r float64) float64 {
		v := 0.0
		for _, row := range s.Rows {
			v += row.Payment / math.Pow(1+r, float64(row.Month))
		}
		return v
	}

	// The payments' worth goes down as the rate goes up.
	lo, hi := 0.0, 1.0
	if worth(hi) > loan {
		return math.Inf(1)
	}
	for range 100 {
		mid := (lo + hi) / 2
		if worth(mid) > loan {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (math.Pow(1+lo, 12) - 1) * 100
}

// FirstPayment is the first monthly payment.
func (s *InstallmentSchedule) FirstPayment() float64 {
	if len(s.Rows) == 0 {
		return 0
	}
	return s.Rows[0].Payment
}

// LastPayment is the last monthly payment, a balloon included.
func (s *InstallmentSchedule) LastPayment() float64 {
	if len(s.Rows) == 0 {
		return 0
	}
	return s.Rows[len(s.Rows)-1].Payment
}

// MaxPayment is the highest monthly payment, a balloon excluded.
func (s *InstallmentSchedule) MaxPayment() float64 {
	m := 0.0
	for i, row := range s.Rows {
		if i == len(s.Rows)-1 && s.Plan.BalloonPct > 0 && len(s.Rows) > 1 {
			continue
		}
		m = max(m, row.Payment)
	}
	return m
}

// IsEven tells if every monthly payment is the same, a balloon aside.
func (s *InstallmentSchedule) IsEven() bool {
	first := s.FirstPayment()
	for i, row := range s.Rows {
		if i == len(s.Rows)-1 && s.Plan.BalloonPct > 0 {
			continue
		}
		if math.Abs(row.Payment-first) > 0.01 {
			return false
		}
	}
	return true
}

// PaidOnDay is what's paid on the purchase day: the down payment and the
// fees not added to the loan.
func (s *InstallmentSchedule) PaidOnDay() float64 {
	return s.DownPayment + s.UpfrontFees
}

// Extra is what the plan costs over the price.
func (s *InstallmentSchedule) Extra() float64 {
	return s.TotalPaid - s.Price
}

// Validate checks the plan's fields and that it works out for the price,
// keyed by field like the validator's errors.
func (p InstallmentPlan) Validate(price float64) errs.ValidationErrors {
	ve := errs.ValidationErrors{}
	inRange := func(key string, v, lo, hi float64) {
		switch {
		case v < lo:
			ve[key] = errs.ValidationField{Tag: "gte", Param: formatParam(lo)}
		case v > hi:
			ve[key] = errs.ValidationField{Tag: "lte", Param: formatParam(hi)}
		}
	}

	inRange("Installments", float64(p.Months), 0, 600)
	inRange("InstallmentAmount", p.Amount, 0, math.Inf(1))
	inRange("Rate", p.Rate, 0, 100)
	inRange("DownPayment", p.DownPayment, 0, pctOr(p.DownPaymentPct, math.Inf(1)))
	inRange("AdminFee", p.AdminFee, 0, pctOr(p.AdminFeePct, math.Inf(1)))
	inRange("MonthlyFee", p.MonthlyFee, 0, math.Inf(1))
	inRange("InsuranceRate", p.InsuranceRate, 0, 100)
	inRange("BalloonPct", p.BalloonPct, 0, 100)
	inRange("GraceMonths", float64(p.GraceMonths), 0, 60)
	if len(ve) > 0 || price <= 0 {
		return ve
	}

	switch _, err := p.Schedule(price); {
	case errors.Is(err, ErrInstallmentMonths):
		ve["Installments"] = errs.ValidationField{Tag: "required"}
	case errors.Is(err, ErrInstallmentKnownAmount):
		ve["Installments"] = errs.ValidationField{Tag: "months_or_amount"}
	case errors.Is(err, ErrInstallmentDownPayment):
		ve["DownPayment"] = errs.ValidationField{Tag: "down_payment_too_high"}
	case errors.Is(err, ErrInstallmentTooLow):
		ve["InstallmentAmount"] = errs.ValidationField{Tag: "amount_too_low"}
	}
	return ve
}

func pctOr(isPct bool, v float64) float64 {
	if isPct {
		return 100
	}
	return v
}

func formatParam(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Summary sums the schedule up, e.g. "12 × E£ 1,100" or "12 mo, E£ 1,200 →
// E£ 850", with what's paid on the day and a balloon if any.
func (s *InstallmentSchedule) Summary(sym string) string {
	n := len(s.Rows)
	var text string
	if s.IsEven() {
		text = fmt.Sprintf("%d × %s", n, formatters.FormatAmount(s.FirstPayment(), sym))
	} else {
		last := s.LastPayment()
		if s.Plan.BalloonPct > 0 && n > 1 {
			last = s.Rows[n-2].Payment
		}
		text = fmt.Sprintf("%d mo, %s → %s", n, formatters.FormatAmount(s.FirstPayment(), sym), formatters.FormatAmount(last, sym))
	}
	if s.Plan.BalloonPct > 0 {
		text += fmt.Sprintf(", last %s", formatters.FormatAmount(s.LastPayment(), sym))
	}
	if d := s.PaidOnDay(); d > 0 {
		text = fmt.Sprintf("%s on the day + %s", formatters.FormatAmount(d, sym), text)
	}
	return text
}

// RoundCents rounds an amount to the nearest cent.
func RoundCents(v float64) float64 {
	return math.Round(v*100) / 100
}
