package models

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
)

// SplitMe is the user among the people sharing an item of a split bill.
const SplitMe = "me"

// SplitEveryone is the share value marking an item shared by everyone in the
// split (TxItem.SharedByAll).
const SplitEveryone = "all"

var ErrSplitNotSplittable = errors.New("only an expense of yours can be split")

// SplitPerson is someone a bill is split with: a counterparty, or the user
// (ID SplitMe).
type SplitPerson struct {
	ID    string
	Name  string
	Icon  string
	Color string
}

func (p *SplitPerson) IsMe() bool { return p.ID == SplitMe }

// CanSplit tells the transaction's bill can be split with people: an expense
// of the user's (not locked, a reimbursement or a transfer's leg).
func (t *Transaction) CanSplit() bool {
	return t.Kind == ExpenseTxKind && !t.IsLocked() && !t.IsTransferLeg() && !t.IsReimbursement()
}

// TxSplit is a transaction's bill split between the user and People, by its
// lines' shares (TxItem.SharedByAll / SharedWith).
type TxSplit struct {
	Tx     *Transaction
	People []SplitPerson
}

// SplitLine is someone's part of a line of the bill.
type SplitLine struct {
	Label string
	// Total is the whole line's, Part its share of it.
	Total, Part float64
	// Of is how many share it (items), 0 for a charge.
	Of int
	// Units is how many of the item's Qty units are theirs, when shared by
	// units (ItemByUnits).
	Units, Qty float64
}

// ItemShare is someone sharing an item of a split bill (Person a
// counterparty id, SplitMe for the user), with their units of it.
type ItemShare struct {
	Person string
	Units  float64
}

// maxSplitUnits is the most units an item is shared by.
const maxSplitUnits = 99

// ItemByUnits tells the item is shared by its units (a whole quantity of 2 or
// more, e.g. 2 of 3 burgers), else by who had it, equally.
func ItemByUnits(it *TxItem) bool {
	return !it.Kind.IsCharge() && it.Quantity > 1 && it.Quantity <= maxSplitUnits && it.Quantity == math.Trunc(it.Quantity)
}

// SplitShare is what someone's part of the bill comes to.
type SplitShare struct {
	Person SplitPerson
	// Items are the items they share, Charges their part of each charge.
	Items, Charges []SplitLine
	Subtotal       float64
	// Total is what they owe, rounded in the bill's currency; the user's
	// takes the rounding, so the shares add up to the lines' total.
	Total float64
	// Requested is what the expense's reimbursements from them add up to,
	// Paid the fulfilled ones' (left out for the user).
	Requested, Paid float64
}

// Left is what's still to be requested from them.
func (s *SplitShare) Left() float64 {
	if s.Person.IsMe() {
		return 0
	}
	return max(s.Total-s.Requested, 0)
}

// SplitResult is a split bill worked out.
type SplitResult struct {
	// Shares are the user's first, then the people's in order.
	Shares []SplitShare
	// Total is the lines' (TxItemsTotal), Mismatch telling it isn't the
	// transaction's amount.
	Total    float64
	Mismatch bool
}

// Others are the people's shares, the user's left out.
func (r *SplitResult) Others() []SplitShare {
	if len(r.Shares) == 0 {
		return nil
	}
	return r.Shares[1:]
}

// ToRequest are the shares with something left to request.
func (r *SplitResult) ToRequest() []SplitShare {
	var out []SplitShare
	for _, s := range r.Others() {
		if s.Left() > 0.005 {
			out = append(out, s)
		}
	}
	return out
}

// chargeByShare tells a charge is split by what each one's items come to
// (a percentage on the prices: VAT, service, discount), else equally per
// head (tip, delivery, fee, rounding).
func chargeByShare(k TxItemKind) bool {
	return k == TxItemVAT || k == TxItemService || k == TxItemDiscount
}

// ChargeSplitRule says how a charge is split, for the views.
func ChargeSplitRule(it *TxItem) string {
	switch {
	case it.Included:
		return "in the prices"
	case chargeByShare(it.Kind):
		return "by share"
	default:
		return "equally"
	}
}

// units are each head's units of the item (the user first, then the
// people): 1 each when shared by everyone; by units, the people's (in the
// split, up to the quantity), the user's the rest; else 1 for each one it's
// shared with still in the split, the user's alone when none.
func (s *TxSplit) units(it *TxItem, at map[string]int, heads int) []float64 {
	u := make([]float64, heads)
	if it.SharedByAll {
		for i := range u {
			u[i] = 1
		}
		return u
	}
	if ItemByUnits(it) {
		left := it.Quantity
		for _, sh := range it.SharedWith {
			if i, ok := at[sh.Person]; ok && i > 0 && sh.Units > 0 {
				n := min(math.Trunc(sh.Units), left)
				u[i] += n
				left -= n
			}
		}
		u[0] = left
		return u
	}
	found := false
	for _, sh := range it.SharedWith {
		if i, ok := at[sh.Person]; ok {
			u[i] = 1
			found = true
		}
	}
	if !found {
		u[0] = 1
	}
	return u
}

// Compute works out everyone's share: each item split equally among who
// shares it, the charges not included in the prices by share or per head
// (chargeByShare).
func (s *TxSplit) Compute() SplitResult {
	cur := &s.Tx.Account.Currency
	heads := 1 + len(s.People)
	shares := make([]SplitShare, heads)
	shares[0].Person = SplitPerson{ID: SplitMe, Name: "You"}
	at := map[string]int{SplitMe: 0}
	for i, p := range s.People {
		shares[i+1].Person = p
		at[p.ID] = i + 1
	}

	raw := make([]float64, heads)
	for i := range s.Tx.Items {
		it := &s.Tx.Items[i]
		if it.Kind.IsCharge() {
			continue
		}
		units := s.units(it, at, heads)
		var all float64
		of := 0
		for _, n := range units {
			all += n
			if n > 0 {
				of++
			}
		}
		for o, n := range units {
			if n <= 0 {
				continue
			}
			part := it.Total * n / all
			ln := SplitLine{Label: it.Label(), Total: it.Total, Part: cur.Round(part), Of: of}
			if ItemByUnits(it) && !it.SharedByAll {
				ln.Units, ln.Qty = n, it.Quantity
			}
			shares[o].Items = append(shares[o].Items, ln)
			shares[o].Subtotal += part
			raw[o] += part
		}
	}

	sub := TxItemsSubtotal(s.Tx.Items)
	for i := range s.Tx.Items {
		it := &s.Tx.Items[i]
		if !it.Kind.IsCharge() || it.Included || it.Total == 0 {
			continue
		}
		for o := range shares {
			part := it.Total / float64(heads)
			if chargeByShare(it.Kind) && math.Abs(sub) > 0.000001 {
				part = it.Total * shares[o].Subtotal / sub
			}
			if math.Abs(part) < 0.000001 {
				continue
			}
			shares[o].Charges = append(shares[o].Charges, SplitLine{Label: it.Label(), Total: it.Total, Part: cur.Round(part)})
			raw[o] += part
		}
	}

	total := cur.Round(TxItemsTotal(s.Tx.Items))
	others := 0.0
	for o := 1; o < heads; o++ {
		shares[o].Subtotal = cur.Round(shares[o].Subtotal)
		shares[o].Total = cur.Round(raw[o])
		others += shares[o].Total
	}
	shares[0].Subtotal = cur.Round(shares[0].Subtotal)
	shares[0].Total = cur.Round(total - others)

	for _, re := range s.Tx.Reimbursements {
		if re.Counterparty == nil {
			continue
		}
		if o, ok := at[re.Counterparty.ID]; ok && o > 0 {
			shares[o].Requested += re.Amount
			if !re.FulfilledAt.IsZero() {
				shares[o].Paid += re.Amount
			}
		}
	}

	return SplitResult{Shares: shares, Total: total, Mismatch: TxItemsMismatch(s.Tx.Items, s.Tx.Amount)}
}

// ItemShares is how the item is shared, for its toggles: SplitEveryone, or
// the ids sharing it (SplitMe when none).
func (s *TxSplit) ItemShares(it *TxItem) []string {
	if it.SharedByAll {
		return []string{SplitEveryone}
	}
	at := s.heads()
	var ids []string
	for i, n := range s.units(it, at, len(s.People)+1) {
		if n > 0 {
			ids = append(ids, s.headID(i))
		}
	}
	return ids
}

// ItemUnits are each one's units of an item shared by units, by id (SplitMe
// the user's, the ones nobody took).
func (s *TxSplit) ItemUnits(it *TxItem) map[string]int {
	out := map[string]int{}
	for i, n := range s.units(&TxItem{Kind: it.Kind, Quantity: it.Quantity, SharedWith: it.SharedWith}, s.heads(), len(s.People)+1) {
		out[s.headID(i)] = int(n)
	}
	return out
}

func (s *TxSplit) heads() map[string]int {
	at := map[string]int{SplitMe: 0}
	for i, p := range s.People {
		at[p.ID] = i + 1
	}
	return at
}

func (s *TxSplit) headID(i int) string {
	if i == 0 {
		return SplitMe
	}
	return s.People[i-1].ID
}

// SplitItemShare is how one item is shared, as saved.
type SplitItemShare struct {
	All  bool
	With []ItemShare
}

// ParseSplitItemShare reads an item's share values (SplitEveryone, SplitMe
// or counterparty ids, with ":<units>" when by units), keeping only the
// people in the split. Everyone wins over the rest. Shared by its units (qty
// > 0, ItemByUnits), the people's units are kept up to qty, the user having
// the rest; else only the user is the same as nobody.
func ParseSplitItemShare(values []string, people []string, qty float64) SplitItemShare {
	if slices.Contains(values, SplitEveryone) {
		return SplitItemShare{All: true}
	}
	var with []ItemShare
	left := qty
	for _, v := range values {
		id, n, hasUnits := strings.Cut(v, ":")
		known := id == SplitMe || slices.Contains(people, id)
		if !known || slices.ContainsFunc(with, func(sh ItemShare) bool { return sh.Person == id }) {
			continue
		}
		if qty <= 0 {
			with = append(with, ItemShare{Person: id, Units: 1})
			continue
		}
		units, err := strconv.Atoi(n)
		if id == SplitMe || !hasUnits || err != nil || units <= 0 || left <= 0 {
			continue
		}
		u := min(float64(units), left)
		left -= u
		with = append(with, ItemShare{Person: id, Units: u})
	}
	if qty <= 0 && len(with) == 1 && with[0].Person == SplitMe {
		return SplitItemShare{}
	}
	return SplitItemShare{With: with}
}
