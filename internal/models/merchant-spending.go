package models

import (
	"math"
	"time"
)

const merchantSpendingTicks = 3

type MerchantSpend struct {
	Counterparty Counterparty
	Amount       float64
}

// MerchantSpending is a month's expenses per merchant, converted to Currency
// and sorted from most to least expensive.
type MerchantSpending struct {
	Month time.Time
	// Prev and Next are the closest months (YYYY-MM) with spending, if any.
	Prev, Next string
	Currency   Currency
	Items      []MerchantSpend
}

// AxisMax is the top of the y axis, rounded up to a readable step.
func (ms *MerchantSpending) AxisMax() float64 {
	var max float64
	for _, it := range ms.Items {
		max = math.Max(max, it.Amount)
	}
	return AxisMax(max, merchantSpendingTicks)
}

// Ticks returns the y axis labels from top to bottom, excluding zero.
func (ms *MerchantSpending) Ticks() []float64 {
	return AxisTicks(ms.AxisMax(), merchantSpendingTicks)
}

// Pct is the amount's height as a percentage of the y axis.
func (ms *MerchantSpending) Pct(amount float64) float64 {
	axisMax := ms.AxisMax()
	if axisMax == 0 {
		return 0
	}
	return amount / axisMax * 100
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
