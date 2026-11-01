package models

import (
	"errors"
	"math"
	"time"
)

// A transfer between the user's own accounts often comes as two SMS, minutes
// or hours apart: a debit read as an expense from one account and a credit
// read as an income to another. They're paired (each pointing at the other)
// so they show as one transfer and stay out of every analysis.
const (
	// TransferPairWindow is how far apart the two legs can happen.
	TransferPairWindow = 24 * time.Hour
	// transferPairFeeRate and transferPairFeeMin bound the fees taken out
	// of the incoming leg: the larger of the two.
	transferPairFeeRate = 0.005
	transferPairFeeMin  = 10.0
	// transferPairFxFeeRate is the bound when the legs are in different
	// currencies, the rates drift.
	transferPairFxFeeRate = 0.05
	transferPairEpsilon   = 0.005
)

// IsTransferLeg reports whether the transaction is half of a transfer pair.
func (t *Transaction) IsTransferLeg() bool {
	return t.TransferPairID != ""
}

// TransferFee is what got lost between the outgoing leg and its incoming
// one, in the outgoing leg's currency (0 when unknown).
func (t *Transaction) TransferFee() float64 {
	in := t.TransferPair
	if in == nil {
		return 0
	}
	rate := 1.0
	if in.Account.Currency.String != t.Account.Currency.String {
		rate = in.Account.Currency.FxRate(t.Account.Currency.String)
		if rate == 0 {
			return 0
		}
	}
	return max(t.Amount-in.Amount*rate, 0)
}

// RateFunc returns the rate converting from a currency to another, 0 when
// unknown.
type RateFunc func(from, to string) float64

var (
	ErrTransferPairKinds   = errors.New("only an expense and an income can be paired as a transfer")
	ErrTransferPairAccount = errors.New("both sides of a transfer must be in different accounts")
	ErrTransferPairNotDone = errors.New("only fulfilled transactions can be paired as a transfer")
	ErrTransferPairLinked  = errors.New("goal transactions, reimbursements and reimbursed expenses can't be paired as a transfer")
	ErrTransferPairAlready = errors.New("one of them is already paired with another transaction")
)

// ValidateTransferPair checks out (an expense) and in (an income) can be
// paired as the two sides of a transfer between the user's accounts, whatever
// their amounts and dates, as when pairing them by hand.
func ValidateTransferPair(out, in *Transaction) error {
	if out.Kind != ExpenseTxKind || in.Kind != IncomeTxKind {
		return ErrTransferPairKinds
	}
	if out.Account.ID == in.Account.ID || out.ID == in.ID {
		return ErrTransferPairAccount
	}
	for _, t := range []*Transaction{out, in} {
		if t.FulfilledAt.IsZero() {
			return ErrTransferPairNotDone
		}
		if t.Goal != nil || t.IsReimbursement() || t.Reimbursed > 0 {
			return ErrTransferPairLinked
		}
		if t.IsTransferLeg() {
			return ErrTransferPairAlready
		}
	}
	return nil
}

// MatchesTransferPair reports whether out (an expense) and in (an income) can
// be the two legs of one transfer between the user's accounts: different
// accounts, fulfilled within TransferPairWindow, and in being out minus some
// fee.
func MatchesTransferPair(out, in *Transaction, rate RateFunc) bool {
	if ValidateTransferPair(out, in) != nil {
		return false
	}
	for _, t := range []*Transaction{out, in} {
		if t.Counterparty != nil && t.Counterparty.IsMerchant {
			return false
		}
	}
	if d := out.FulfilledAt.Sub(in.FulfilledAt); d > TransferPairWindow || d < -TransferPairWindow {
		return false
	}

	inAmount, feeRate := in.Amount, transferPairFeeRate
	if from, to := in.Account.Currency.String, out.Account.Currency.String; from != to {
		r := rate(from, to)
		if r == 0 {
			return false
		}
		inAmount, feeRate = inAmount*r, transferPairFxFeeRate
	}

	fee := out.Amount - inAmount
	if fee < -transferPairEpsilon && feeRate == transferPairFeeRate {
		return false
	}
	return math.Abs(fee) <= max(out.Amount*feeRate, transferPairFeeMin)+transferPairEpsilon
}

// PickTransferPair returns the candidate making a transfer pair with tx, the
// closest in time then in amount, or nil when none does.
func PickTransferPair(tx *Transaction, candidates []Transaction, rate RateFunc) *Transaction {
	var best *Transaction
	var bestGap time.Duration
	var bestDiff float64
	for i := range candidates {
		c := &candidates[i]
		out, in := tx, c
		if tx.Kind == IncomeTxKind {
			out, in = c, tx
		}
		if !MatchesTransferPair(out, in, rate) {
			continue
		}
		gap := tx.FulfilledAt.Sub(c.FulfilledAt).Abs()
		diff := math.Abs(out.Amount - in.Amount)
		if best == nil || gap < bestGap || (gap == bestGap && diff < bestDiff) {
			best, bestGap, bestDiff = c, gap, diff
		}
	}
	return best
}
