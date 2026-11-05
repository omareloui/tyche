package models

import (
	"math"
	"time"
)

type MerchantSpend struct {
	Counterparty Counterparty
	Amount       float64
}

// MerchantSpending is a month's expenses per merchant, converted to Currency
// and sorted from most to least expensive.
type MerchantSpending struct {
	// Month is the 1st of the month Period is named after.
	Month  time.Time
	Period Period
	// Prev and Next are the closest months (YYYY-MM) with spending, if any.
	Prev, Next string
	Currency   Currency
	Items      []MerchantSpend
}

// MerchantSpendingTop is how many merchants show before "Show more".
const MerchantSpendingTop = 6

// Total is what was spent at all its merchants.
func (ms *MerchantSpending) Total() float64 {
	var t float64
	for _, it := range ms.Items {
		t += max(it.Amount, 0)
	}
	return t
}

// Share is the amount's percentage of Total.
func (ms *MerchantSpending) Share(amount float64) float64 {
	t := ms.Total()
	if t <= 0 {
		return 0
	}
	return max(amount, 0) / t * 100
}

// BarPct is the amount's bar width, as a percentage of the top merchant's.
func (ms *MerchantSpending) BarPct(amount float64) float64 {
	var top float64
	for _, it := range ms.Items {
		top = max(top, it.Amount)
	}
	if top <= 0 {
		return 0
	}
	return max(amount, 0) / top * 100
}

// niceStep rounds v up to a readable multiple of a power of ten.
func niceStep(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 1.5, 2, 2.5, 3, 4, 5, 6, 8} {
		if v <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}
