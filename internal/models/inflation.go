package models

import (
	"fmt"
	"math"
)

// InflationYear is a year of the inflation calculator: its consumer prices'
// change and what the amount is worth then.
type InflationYear struct {
	Year int
	// Pct is the year's consumer prices' change (%), 0 for the first year.
	Pct float64
	// Value is the amount (of the from year) in that year's money.
	Value float64
}

// InflationResult is what an amount of a year is worth in another, by the
// yearly consumer prices' change (annual averages).
type InflationResult struct {
	From, To int
	Amount   float64
	// Worth is the amount in the to year's money.
	Worth float64
	// Cumulative is the prices' change from one year to the other
	// (Worth/Amount − 1), AvgPct its average a year (%, geometric).
	Cumulative float64
	AvgPct     float64
	// PowerLost is the share of what the from year's money bought that the
	// same amount no longer buys in the to year (negative when it buys more).
	PowerLost float64
	// Years go from the earlier year to the later one.
	Years []InflationYear
}

// Back tells the result goes back in time (to before from).
func (r InflationResult) Back() bool { return r.To < r.From }

// ComputeInflation brings an amount of year from to year to by the yearly
// prices' change (year → %): its value in each year in between is the
// previous one's times the year's change. Every year after the earlier one,
// through the later one, needs a rate.
func ComputeInflation(rates map[int]float64, amount float64, from, to int) (InflationResult, error) {
	r := InflationResult{From: from, To: to, Amount: amount}
	lo, hi := min(from, to), max(from, to)
	for y := lo + 1; y <= hi; y++ {
		if _, ok := rates[y]; !ok {
			return r, fmt.Errorf("no inflation data for %d", y)
		}
	}

	// Prices' index of each year, the earlier one at 1.
	idx := make([]float64, hi-lo+1)
	idx[0] = 1
	for i := 1; i < len(idx); i++ {
		idx[i] = idx[i-1] * (1 + rates[lo+i]/100)
	}
	base := idx[from-lo]
	r.Years = make([]InflationYear, len(idx))
	for i, v := range idx {
		y := InflationYear{Year: lo + i, Value: amount * v / base}
		if i > 0 {
			y.Pct = rates[lo+i]
		}
		r.Years[i] = y
	}

	factor := idx[to-lo] / base
	r.Worth = amount * factor
	r.Cumulative = factor - 1
	r.PowerLost = 1 - 1/factor
	if n := hi - lo; n > 0 {
		// The average over the years walked, signed like the prices went.
		up := idx[len(idx)-1]
		r.AvgPct = (math.Pow(up, 1/float64(n)) - 1) * 100
	}
	return r, nil
}
