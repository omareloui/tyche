package models

import (
	"cmp"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
)

// The FIRE calculator's constants.
const (
	// FireMonths is how many full user months of income and spending the
	// calculator averages.
	FireMonths = 12
	// FireMaxYears is how far ahead the calculator looks for the day the
	// portfolio reaches its number: never past it.
	FireMaxYears = 60
	// FireLifeAge is the age the money has to last to in the simulation,
	// FireHorizonYears how long it has to last without an age, and
	// FireMinHorizonYears at the least.
	FireLifeAge         = 95
	FireHorizonYears    = 40
	FireMinHorizonYears = 10
	// FireChartRetired is how many years after reaching the number the chart
	// goes on; FireChartYears is its length when it's never reached.
	FireChartRetired = 15
	FireChartYears   = 40
	// FireRuns is how many runs the simulation makes.
	FireRuns = 1000
	// FireLeanShare and FireFatShare are the lean / fat FIRE's spending
	// against today's.
	FireLeanShare = 0.75
	FireFatShare  = 1.5
	// FireMinReturn … are the assumptions the calculator takes (fractions).
	FireMinReturn = -0.05
	FireMaxReturn = 0.15
	FireMinSWR    = 0.01
	FireMaxSWR    = 0.10
	FireMaxVol    = 0.40
	// fireFloorReturn is the worst year a run can have.
	fireFloorReturn = -0.95
)

// FireInput is what the FIRE calculator works from, in today's money (real
// terms) of one currency.
type FireInput struct {
	// Portfolio is what's invested (or saved) now, negative when debts
	// outweigh it (saving pays them off first); Spending and Saving are a
	// month's.
	Portfolio, Spending, Saving float64
	// Return is the real return a year, SWR the safe withdrawal rate (a
	// year's spending over the portfolio), Volatility the returns' standard
	// deviation a year: fractions.
	Return, SWR, Volatility float64
	// Age is the user's this year, RetireAge the one a Coast FIRE retires
	// at: 0 = unknown.
	Age, RetireAge int
	// PartTime is a month's income a Barista FIRE keeps working for.
	PartTime float64
}

// Validate tells what's off in the input.
func (in FireInput) Validate() error {
	switch {
	case in.Saving < 0 || in.PartTime < 0:
		return errors.New("amounts can't be negative")
	case in.Spending <= 0:
		return errors.New("enter what you spend a month")
	case in.SWR < FireMinSWR || in.SWR > FireMaxSWR:
		return fmt.Errorf("the withdrawal rate goes from %g%% to %g%%", FireMinSWR*100, FireMaxSWR*100)
	case in.Return < FireMinReturn || in.Return > FireMaxReturn:
		return fmt.Errorf("the return goes from %g%% to %g%%", FireMinReturn*100, FireMaxReturn*100)
	case in.Volatility < 0 || in.Volatility > FireMaxVol:
		return fmt.Errorf("the volatility goes from 0%% to %g%%", FireMaxVol*100)
	case in.Age > 0 && in.RetireAge > 0 && in.RetireAge <= in.Age:
		return errors.New("the retirement age has to be ahead of your age")
	}
	return nil
}

// FireNumber is the portfolio a month's spending needs: a year's of it over
// the withdrawal rate.
func FireNumber(monthly, swr float64) float64 {
	if swr <= 0 {
		return 0
	}
	return monthly * 12 / swr
}

// monthlyRate is the return a month of a yearly one.
func monthlyRate(r float64) float64 { return math.Pow(1+r, 1.0/12) - 1 }

// fireMonthsTo is how many months saving takes the portfolio to target
// (false when not within FireMaxYears).
func fireMonthsTo(portfolio, saving, r, target float64) (int, bool) {
	if portfolio >= target {
		return 0, true
	}
	mr := monthlyRate(r)
	p := portfolio
	for m := 1; m <= FireMaxYears*12; m++ {
		p = p*(1+mr) + saving
		if p >= target {
			return m, true
		}
	}
	return 0, false
}

// FireYear is a year of the projection.
type FireYear struct {
	// Year counts from now (0), Age is the user's then (0 = unknown).
	Year, Age int
	Portfolio float64
	// Retired tells the portfolio reached its number by then.
	Retired bool
}

// FireVariant is a flavor of FIRE: its number and when it's reached.
type FireVariant struct {
	Key, Name, Text string
	Number          float64
	// Months is how long saving takes to it, Never when not within
	// FireMaxYears.
	Months int
	Never  bool
	// Age is the user's then (0 = unknown).
	Age int
}

// Reached tells the portfolio is at the variant's number already.
func (v FireVariant) Reached() bool { return !v.Never && v.Months == 0 }

// FireResult is what the calculator comes to, without the simulation.
type FireResult struct {
	Input  FireInput
	Number float64
	// Months is how long saving takes to the number, Never when not within
	// FireMaxYears; Age the user's then (0 = unknown).
	Months int
	Never  bool
	Age    int
	// Progress is the portfolio's share of the number (within 0 and 1).
	Progress float64
	// Path is the portfolio year by year: saving until the number, then
	// spending out of it.
	Path     []FireYear
	Variants []FireVariant
}

// Years is Months in years.
func (r FireResult) Years() float64 { return float64(r.Months) / 12 }

func fireAge(age, months int) int {
	if age <= 0 {
		return 0
	}
	return age + months/12
}

// ComputeFire works the FIRE number out and when saving reaches it,
// returning at the rate every year.
func ComputeFire(in FireInput) FireResult {
	res := FireResult{Input: in, Number: FireNumber(in.Spending, in.SWR)}
	if res.Number > 0 {
		res.Progress = max(min(in.Portfolio/res.Number, 1), 0)
	}
	m, ok := fireMonthsTo(in.Portfolio, in.Saving, in.Return, res.Number)
	res.Months, res.Never = m, !ok
	if ok {
		res.Age = fireAge(in.Age, m)
	}
	res.Path = firePath(in, res.Number, fireChartYears(res.Months, res.Never))
	res.Variants = fireVariants(in, res)
	return res
}

// fireChartYears is how many years the chart shows.
func fireChartYears(months int, never bool) int {
	if never {
		return FireChartYears
	}
	return max(min(int(math.Ceil(float64(months)/12))+FireChartRetired, FireMaxYears), FireMinHorizonYears)
}

// firePath steps the portfolio month by month: saving until it reaches
// target, then spending out of it.
func firePath(in FireInput, target float64, years int) []FireYear {
	mr := monthlyRate(in.Return)
	p, retired := in.Portfolio, in.Portfolio >= target
	path := make([]FireYear, 0, years+1)
	path = append(path, FireYear{Year: 0, Age: in.Age, Portfolio: p, Retired: retired})
	for y := 1; y <= years; y++ {
		for range 12 {
			if retired {
				p = max(p*(1+mr)-in.Spending, 0)
				continue
			}
			p = p*(1+mr) + in.Saving
			retired = p >= target
		}
		age := 0
		if in.Age > 0 {
			age = in.Age + y
		}
		path = append(path, FireYear{Year: y, Age: age, Portfolio: p, Retired: retired})
	}
	return path
}

func fireVariants(in FireInput, res FireResult) []FireVariant {
	variant := func(key, name, text string, monthly float64) FireVariant {
		v := FireVariant{Key: key, Name: name, Text: text, Number: FireNumber(monthly, in.SWR)}
		m, ok := fireMonthsTo(in.Portfolio, in.Saving, in.Return, v.Number)
		v.Months, v.Never = m, !ok
		if ok {
			v.Age = fireAge(in.Age, m)
		}
		return v
	}
	vs := []FireVariant{
		variant("lean", "Lean FIRE", fmt.Sprintf("Living on %g%% of what you spend now.", FireLeanShare*100), in.Spending*FireLeanShare),
		{Key: "fire", Name: "FIRE", Text: "Living like you do now.", Number: res.Number, Months: res.Months, Never: res.Never, Age: res.Age},
		variant("fat", "Fat FIRE", fmt.Sprintf("Living on %g%% of what you spend now.", FireFatShare*100), in.Spending*FireFatShare),
	}
	if in.PartTime > 0 && in.PartTime < in.Spending {
		vs = append(vs, variant("barista", "Barista FIRE", "Working part-time for the rest of what you spend.", in.Spending-in.PartTime))
	}
	if c, ok := fireCoast(in, res.Number); ok {
		vs = append(vs, c)
	}
	return vs
}

// fireCoast is the Coast FIRE: the portfolio that, without saving more,
// grows to the number by the retirement age. Its number is today's; Months
// is when saving gets there (the number it needs shrinks as it nears).
func fireCoast(in FireInput, number float64) (FireVariant, bool) {
	if in.Age <= 0 || in.RetireAge <= in.Age {
		return FireVariant{}, false
	}
	years := in.RetireAge - in.Age
	need := func(monthsIn int) float64 {
		left := float64(years) - float64(monthsIn)/12
		if left <= 0 {
			return number
		}
		return number / math.Pow(1+in.Return, left)
	}
	v := FireVariant{
		Key: "coast", Name: "Coast FIRE",
		Text:   fmt.Sprintf("Enough that, saving nothing more, it grows to your FIRE number by %d.", in.RetireAge),
		Number: need(0), Never: true,
	}
	mr := monthlyRate(in.Return)
	p := in.Portfolio
	for m := 0; m <= min(years, FireMaxYears)*12; m++ {
		if m > 0 {
			p = p*(1+mr) + in.Saving
		}
		if p >= need(m) {
			v.Months, v.Never, v.Age = m, false, fireAge(in.Age, m)
			break
		}
	}
	return v, true
}

// FireBand is a year of the simulation: its runs' portfolios' 10th, 50th
// and 90th percentiles.
type FireBand struct {
	Year          int
	P10, P50, P90 float64
}

// FireSim is what the simulation comes to.
type FireSim struct {
	Runs int
	// ReachRate is the runs' share reaching the number within FireMaxYears.
	ReachRate float64
	// SuccessRate is the share of the runs reaching it whose money lasts
	// the horizon after.
	SuccessRate float64
	// Years are the 10th, 50th and 90th percentiles of the years to the
	// number, -1 = not within FireMaxYears.
	Years [3]int
	// Horizon is how many years the money has to last after (with the
	// median's age).
	Horizon int
	Bands   []FireBand
}

// FireSeed is the simulation's seed for an input, so the same input always
// comes to the same result.
func FireSeed(in FireInput) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%v", in)
	return h.Sum64()
}

// fireHorizon is how many years the money has to last once retired at age
// (0 = unknown).
func fireHorizon(age int) int {
	if age <= 0 {
		return FireHorizonYears
	}
	return max(FireLifeAge-age, FireMinHorizonYears)
}

// SimulateFire runs the plan through random years: each year's real return
// drawn from a normal of Return and Volatility. A run saves (a year's at the
// year's end) until the number, then spends a year's out of the portfolio
// at the start of each year for the horizon after; it succeeds when the
// money lasts. Bands covers chartYears years.
func SimulateFire(in FireInput, runs, chartYears int, seed uint64) FireSim {
	sim := FireSim{Runs: runs}
	if runs <= 0 {
		return sim
	}
	target := FireNumber(in.Spending, in.SWR)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	ret := func() float64 {
		return max(in.Return+rng.NormFloat64()*in.Volatility, fireFloorReturn)
	}

	paths := make([][]float64, chartYears+1)
	for y := range paths {
		paths[y] = make([]float64, runs)
	}
	years := make([]int, runs)
	reached, lasted := 0, 0
	for i := range runs {
		p := in.Portfolio
		fi := -1
		if p >= target {
			fi = 0
		}
		paths[0][i] = p
		y := 0
		for ; fi < 0 && y < FireMaxYears; y++ {
			p = p*(1+ret()) + in.Saving*12
			if y+1 <= chartYears {
				paths[y+1][i] = p
			}
			if p >= target {
				fi = y + 1
			}
		}
		years[i] = fi
		if fi < 0 {
			continue
		}
		reached++
		horizon := fireHorizon(fireAge(in.Age, fi*12))
		ok := true
		for k := 0; k < horizon || fi+k < chartYears; k++ {
			if p > 0 {
				p = max((p-in.Spending*12)*(1+ret()), 0)
			}
			if k < horizon && p <= 0 {
				ok = false
			}
			if fi+k+1 <= chartYears {
				paths[fi+k+1][i] = p
			}
		}
		if ok {
			lasted++
		}
	}

	sim.ReachRate = float64(reached) / float64(runs)
	if reached > 0 {
		sim.SuccessRate = float64(lasted) / float64(reached)
	}
	sorted := slices.Clone(years)
	slices.SortFunc(sorted, func(a, b int) int {
		// Never sorts last.
		if a < 0 {
			a = math.MaxInt
		}
		if b < 0 {
			b = math.MaxInt
		}
		return cmp.Compare(a, b)
	})
	for k, q := range []float64{0.1, 0.5, 0.9} {
		sim.Years[k] = sorted[min(int(q*float64(runs)), runs-1)]
	}
	median := sim.Years[1]
	if median < 0 {
		median = FireMaxYears
	}
	sim.Horizon = fireHorizon(fireAge(in.Age, median*12))
	sim.Bands = make([]FireBand, len(paths))
	for y, ps := range paths {
		slices.Sort(ps)
		at := func(q float64) float64 { return ps[min(int(q*float64(runs)), runs-1)] }
		sim.Bands[y] = FireBand{Year: y, P10: at(0.1), P50: at(0.5), P90: at(0.9)}
	}
	return sim
}

// FireMoney is the user's money the FIRE calculator starts from, in the
// default currency.
type FireMoney struct {
	// Portfolio is the accounts' money (see GetFireMoney).
	Portfolio float64
	// Excluded and Goals count the accounts out of the balance and the
	// goals', whichever are in Portfolio.
	Excluded, Goals int
	// Income and Spending are the last FireMonths full user months', oldest
	// first; Periods their months.
	Income, Spending []float64
	Periods          []Period
	Partial          bool
}

// active is the index of the first month with any money moved: the months
// before it (before the user started) don't count.
func (m *FireMoney) active() int {
	for i := range m.Income {
		if m.Income[i] > 0 || m.Spending[i] > 0 {
			return i
		}
	}
	return len(m.Income)
}

// Months is how many months the averages are of.
func (m *FireMoney) Months() int { return len(m.Income) - m.active() }

func avgFrom(vs []float64, from int) float64 {
	if from >= len(vs) {
		return 0
	}
	var sum float64
	for _, v := range vs[from:] {
		sum += v
	}
	return sum / float64(len(vs)-from)
}

// AvgIncome and AvgSpending are a month's on average, over the months since
// the first with money moved.
func (m *FireMoney) AvgIncome() float64   { return avgFrom(m.Income, m.active()) }
func (m *FireMoney) AvgSpending() float64 { return avgFrom(m.Spending, m.active()) }
