package statementocr

import (
	"math"
	"time"
)

// FeeDays is how far apart (days) a fee line and its purchase can be.
const FeeDays = 3

// FxFeeMerge is what MergeFxFees did.
type FxFeeMerge struct {
	// Merged fee lines went into their purchase (or refund); Unmatched ones
	// are kept as rows of their own.
	Merged, Unmatched int
	// Percent is the fee matched with: the account's, else the one most fee
	// lines are of their purchases (Inferred), 0 when none could be told.
	Percent  float64
	Inferred bool
}

// MergeFxFees adds each foreign exchange fee line (IsFxFee) to the
// purchase it was charged on, the row it's percent of (within a cent, or 1%
// of the fee), on the same side, closest in days (≤ FeeDays): the purchase
// then carries it (Debit and FxFee), a fee's reversal goes into its refund's
// Credit, and the fee line is dropped. percent is the account's fee (0 to
// tell it from the lines). Lines without their purchase stay rows.
func (r *Result) MergeFxFees(percent float64) FxFeeMerge {
	res := FxFeeMerge{Percent: percent}
	var fees, others []int
	for i, row := range r.Rows {
		if row.IsFxFee && row.Debit+row.Credit > 0 {
			fees = append(fees, i)
		} else {
			others = append(others, i)
		}
	}
	if len(fees) == 0 {
		return res
	}
	if res.Percent <= 0 {
		res.Percent, res.Inferred = r.inferFeePercent(fees, others), true
	}
	if res.Percent <= 0 {
		res.Unmatched = len(fees)
		return res
	}

	used := map[int]bool{}
	drop := map[int]bool{}
	for _, fi := range fees {
		fee := r.Rows[fi]
		out := fee.Debit > 0
		amount := max(fee.Debit, fee.Credit)
		best, bestDays, bestOff := -1, FeeDays+1, math.Inf(1)
		for _, oi := range others {
			o := r.Rows[oi]
			base := o.Credit
			if out {
				base = o.Debit
			}
			if used[oi] || base <= 0 {
				continue
			}
			off := math.Abs(base*res.Percent/100 - amount)
			if off > max(0.011, amount*0.01) {
				continue
			}
			days := dayGap(fee.Date, o.Date)
			if days < 0 || days > FeeDays {
				continue
			}
			if days < bestDays || (days == bestDays && off < bestOff) {
				best, bestDays, bestOff = oi, days, off
			}
		}
		if best < 0 {
			res.Unmatched++
			continue
		}
		used[best] = true
		drop[fi] = true
		res.Merged++
		p := &r.Rows[best]
		if out {
			p.Debit = round2(p.Debit + amount)
			p.FxFee = round2(p.FxFee + amount)
		} else {
			p.Credit = round2(p.Credit + amount)
		}
	}
	kept := r.Rows[:0]
	for i, row := range r.Rows {
		if !drop[i] {
			kept = append(kept, row)
		}
	}
	r.Rows = kept
	return res
}

// inferFeePercent is the fee percent (to 0.1, within 0.5–10) most fee lines
// are of a same day row on their side, 0 when none is.
func (r *Result) inferFeePercent(fees, others []int) float64 {
	counts := map[float64]int{}
	for _, fi := range fees {
		fee := r.Rows[fi]
		out := fee.Debit > 0
		amount := max(fee.Debit, fee.Credit)
		seen := map[float64]bool{}
		for _, oi := range others {
			o := r.Rows[oi]
			base := o.Credit
			if out {
				base = o.Debit
			}
			if base <= 0 || dayGap(fee.Date, o.Date) != 0 {
				continue
			}
			pct := math.Round(amount/base*1000) / 10
			if pct >= 0.5 && pct <= 10 && !seen[pct] {
				seen[pct] = true
				counts[pct]++
			}
		}
	}
	best, bestN := 0.0, 0
	for pct, n := range counts {
		if n > bestN || (n == bestN && pct < best) {
			best, bestN = pct, n
		}
	}
	if bestN < 2 && len(fees) > 1 {
		return 0
	}
	return best
}

// dayGap is how many days apart two YYYY-MM-DD days are, -1 when either
// doesn't parse.
func dayGap(a, b string) int {
	da, err1 := time.Parse(time.DateOnly, a)
	db, err2 := time.Parse(time.DateOnly, b)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(math.Abs(math.Round(da.Sub(db).Hours() / 24)))
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
