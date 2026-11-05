package models

import (
	"math"
	"time"
)

// RecurrentMatchWindow is how far an SMS can be from an occurrence of a
// recurrent transaction to be taken for its payment on its own.
const RecurrentMatchWindow = 5 * 24 * time.Hour

// RecurrentManualMatchWindow is how far the occurrences offered to match an
// SMS transaction by hand can be.
const RecurrentManualMatchWindow = 14 * 24 * time.Hour

// RecurrentMatchTolerance is how far off an SMS's amount can be from the
// occurrence's when their counterparties don't tell.
const RecurrentMatchTolerance = 0.10

// paysKind tells whether an SMS transaction of kind sms can be the payment of
// an occurrence of kind occ: a debit pays an expense or a transfer out of the
// account, a credit an income.
func paysKind(sms, occ TxKind) bool {
	switch sms {
	case ExpenseTxKind:
		return occ == ExpenseTxKind || occ == TransferTxKind
	case IncomeTxKind:
		return occ == IncomeTxKind
	}
	return false
}

// MayPayOccurrence tells whether the SMS transaction sms can be the payment
// of occ (an occurrence of a recurrent transaction) at all: same account and
// a kind paying it.
func MayPayOccurrence(sms, occ *Transaction) bool {
	return sms.Account.ID == occ.Account.ID && paysKind(sms.Kind, occ.Kind)
}

// MatchesRecurrentOccurrence tells whether the SMS transaction sms is likely
// the payment of occ: within RecurrentMatchWindow of it, of its counterparty
// or about its amount.
func MatchesRecurrentOccurrence(sms, occ *Transaction) bool {
	if !MayPayOccurrence(sms, occ) {
		return false
	}
	if d := sms.ScheduledTo.Sub(occ.ScheduledTo); d < -RecurrentMatchWindow || d > RecurrentMatchWindow {
		return false
	}
	sameCp := sms.Counterparty != nil && occ.Counterparty != nil && sms.Counterparty.ID == occ.Counterparty.ID
	closeAmount := occ.Amount > 0 && math.Abs(sms.Amount-occ.Amount) <= occ.Amount*RecurrentMatchTolerance
	return sameCp || closeAmount
}

// PickRecurrentOccurrence is the candidate sms most likely pays: the closest
// in date of the matching ones, then in amount. Nil if none matches.
func PickRecurrentOccurrence(sms *Transaction, candidates []Transaction) *Transaction {
	var best *Transaction
	var bestDays, bestAmount float64
	for i := range candidates {
		c := &candidates[i]
		if !MatchesRecurrentOccurrence(sms, c) {
			continue
		}
		days := math.Abs(sms.ScheduledTo.Sub(c.ScheduledTo).Hours()) / 24
		amount := math.Abs(sms.Amount - c.Amount)
		if best == nil || days < bestDays || (days == bestDays && amount < bestAmount) {
			best, bestDays, bestAmount = c, days, amount
		}
	}
	return best
}

// MergeSMS makes the occurrence the SMS transaction sms paid: the amount
// and date it was paid on and fulfilled, flagged for review, keeping what the
// recurrent transaction set (category, counterparty, note), filled in from
// the SMS where it set none.
func (t *Transaction) MergeSMS(sms *Transaction) {
	if t.Kind == TransferTxKind && t.Amount > 0 {
		t.ToAmount = t.ToAmount * sms.Amount / t.Amount
	}
	t.Amount = sms.Amount
	t.ScheduledTo = sms.ScheduledTo
	t.FulfilledAt = sms.FulfilledAt
	t.NeedsReview = true
	if t.Kind != TransferTxKind {
		if t.Counterparty == nil {
			t.Counterparty = sms.Counterparty
		}
		if t.Category == nil {
			t.Category = sms.Category
		}
	}
	if t.Description == "" {
		t.Description = sms.Description
	}
}
