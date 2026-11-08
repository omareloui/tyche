package models

import (
	"math"
	"slices"
	"testing"
	"time"
)

func splitTx(items []TxItem, amount float64) *Transaction {
	return &Transaction{Kind: ExpenseTxKind, Amount: amount, Items: items}
}

// with shares an item with ids, 1 unit each.
func with(ids ...string) []ItemShare {
	out := make([]ItemShare, len(ids))
	for i, id := range ids {
		out[i] = ItemShare{Person: id, Units: 1}
	}
	return out
}

func shareTotals(r SplitResult) map[string]float64 {
	out := map[string]float64{}
	for _, s := range r.Shares {
		out[s.Person.ID] = s.Total
	}
	return out
}


func TestTxSplitCompute(t *testing.T) {
	ali := SplitPerson{ID: "ali", Name: "Ali"}
	sara := SplitPerson{ID: "sara", Name: "Sara"}

	tests := []struct {
		name   string
		items  []TxItem
		people []SplitPerson
		want   map[string]float64
	}{
		{
			name:   "unassigned items are the user's",
			items:  []TxItem{{Name: "Burger", Total: 200}, {Name: "Juice", Total: 50, SharedWith: with("ali")}},
			people: []SplitPerson{ali},
			want:   map[string]float64{SplitMe: 200, "ali": 50},
		},
		{
			name:   "an item shared by some splits equally among them",
			items:  []TxItem{{Name: "Pizza", Total: 300, SharedWith: with(SplitMe, "sara")}, {Name: "Soda", Total: 30, SharedWith: with("ali")}},
			people: []SplitPerson{ali, sara},
			want:   map[string]float64{SplitMe: 150, "ali": 30, "sara": 150},
		},
		{
			name:   "an item for the table is everyone's, also the ones added later",
			items:  []TxItem{{Name: "Fries", Total: 90, SharedByAll: true}},
			people: []SplitPerson{ali, sara},
			want:   map[string]float64{SplitMe: 30, "ali": 30, "sara": 30},
		},
		{
			name: "percentage charges go by share, flat ones per head",
			items: []TxItem{
				{Name: "Steak", Total: 300, SharedWith: with("ali")},
				{Name: "Salad", Total: 100},
				{Kind: TxItemService, Rate: ptr(12), Total: 48},
				{Kind: TxItemVAT, Rate: ptr(14), Total: 62.72},
				{Kind: TxItemDiscount, Total: -40},
				{Kind: TxItemTip, Total: 20},
				{Kind: TxItemDelivery, Total: 10},
			},
			people: []SplitPerson{ali},
			// Ali: 300 + 36 + 47.04 - 20 + 10 + 5 (the discount without a
			// rate per head); the user the rest.
			want: map[string]float64{SplitMe: 122.68, "ali": 378.04},
		},
		{
			name: "a charge's toggle wins over its default",
			items: []TxItem{
				{Name: "Steak", Total: 300, SharedWith: with("ali")},
				{Name: "Salad", Total: 100},
				{Kind: TxItemVAT, Rate: ptr(14), Total: 56, SplitMode: ChargeSplitEqually},
				{Kind: TxItemDelivery, Total: 40, SplitMode: ChargeSplitByShare},
			},
			people: []SplitPerson{ali},
			// Ali: 300 + 28 + 30; the user 100 + 28 + 10.
			want: map[string]float64{SplitMe: 138, "ali": 358},
		},
		{
			name: "charges included in the prices add nothing",
			items: []TxItem{
				{Name: "Coffee", Total: 60, SharedWith: with("ali")},
				{Name: "Cake", Total: 40},
				{Kind: TxItemVAT, Rate: ptr(14), Total: 14, Included: true},
			},
			people: []SplitPerson{ali},
			want:   map[string]float64{SplitMe: 40, "ali": 60},
		},
		{
			name:   "the user takes the rounding",
			items:  []TxItem{{Name: "Platter", Total: 100, SharedByAll: true}},
			people: []SplitPerson{ali, sara},
			want:   map[string]float64{SplitMe: 33.34, "ali": 33.33, "sara": 33.33},
		},
		{
			name:   "an item with a quantity is shared by its units, the rest the user's",
			items:  []TxItem{{Name: "Burger", Quantity: 3, UnitPrice: 100, Total: 300, SharedWith: []ItemShare{{"ali", 1}, {"sara", 1}}}},
			people: []SplitPerson{ali, sara},
			want:   map[string]float64{SplitMe: 100, "ali": 100, "sara": 100},
		},
		{
			name:   "units past the quantity are dropped",
			items:  []TxItem{{Name: "Beer", Quantity: 4, UnitPrice: 50, Total: 200, SharedWith: []ItemShare{{"ali", 3}, {"sara", 2}}}},
			people: []SplitPerson{ali, sara},
			want:   map[string]float64{SplitMe: 0, "ali": 150, "sara": 50},
		},
		{
			name:   "an item with a quantity can still be everyone's",
			items:  []TxItem{{Name: "Bread", Quantity: 2, UnitPrice: 15, Total: 30, SharedByAll: true}},
			people: []SplitPerson{ali},
			want:   map[string]float64{SplitMe: 15, "ali": 15},
		},
		{
			name:   "shares of people not in the split anymore go back to the user",
			items:  []TxItem{{Name: "Tea", Total: 25, SharedWith: with("gone")}, {Name: "Water", Total: 10, SharedWith: with("gone", "ali")}},
			people: []SplitPerson{ali},
			want:   map[string]float64{SplitMe: 25, "ali": 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := TxSplit{Tx: splitTx(tt.items, TxItemsTotal(tt.items)), People: tt.people}
			r := s.Compute()
			got := shareTotals(r)
			var sum float64
			for id, w := range tt.want {
				if !near(got[id], w) {
					t.Errorf("%s owes %v, want %v", id, got[id], w)
				}
			}
			for _, sh := range r.Shares {
				sum += sh.Total
			}
			if !near(math.Round(sum*100)/100, r.Total) {
				t.Errorf("shares add up to %v, want the lines' %v", sum, r.Total)
			}
			if r.Mismatch {
				t.Error("mismatch, want none")
			}
		})
	}
}

func TestTxSplitRequested(t *testing.T) {
	ali := &Counterparty{ID: "ali"}
	tx := splitTx([]TxItem{{Name: "Pizza", Total: 200, SharedByAll: true}}, 210)
	tx.Reimbursements = []Transaction{
		{Amount: 60, Counterparty: ali, FulfilledAt: time.Now()},
		{Amount: 20, Counterparty: ali},
		{Amount: 5}, // no counterparty, nobody's
	}
	s := TxSplit{Tx: tx, People: []SplitPerson{{ID: "ali"}, {ID: "sara"}}}
	r := s.Compute()
	if !r.Mismatch {
		t.Error("no mismatch, want the lines (200) not adding up to the amount (210)")
	}
	// 200 / 3: 66.67 each, the user 66.66.
	a := r.Shares[1]
	if a.Requested != 80 || a.Paid != 60 {
		t.Errorf("ali requested %v paid %v, want 80 and 60", a.Requested, a.Paid)
	}
	if a.Left() != 0 {
		t.Errorf("ali left %v, want 0 (requested more than the share)", a.Left())
	}
	req := r.ToRequest()
	if len(req) != 1 || req[0].Person.ID != "sara" || !near(req[0].Left(), 66.67) {
		t.Errorf("to request = %+v, want sara's 66.67", req)
	}
	if !near(r.Shares[0].Total, 66.66) {
		t.Errorf("the user's share = %v, want 66.66", r.Shares[0].Total)
	}
}

func TestParseSplitItemShare(t *testing.T) {
	people := []string{"ali", "sara"}
	tests := []struct {
		name   string
		values []string
		qty    float64
		want   SplitItemShare
	}{
		{"nobody is the user's", nil, 0, SplitItemShare{}},
		{"the user alone is kept, as picked", []string{SplitMe}, 0, SplitItemShare{With: with(SplitMe)}},
		{"everyone wins", []string{"ali", SplitEveryone}, 0, SplitItemShare{All: true}},
		{"people not in the split are dropped", []string{"ali", "bob", "ali"}, 0, SplitItemShare{With: with("ali")}},
		{"the user with others", []string{SplitMe, "sara"}, 0, SplitItemShare{With: with(SplitMe, "sara")}},
		{"units up to the quantity", []string{"ali:2", "sara:3", SplitMe + ":1"}, 3, SplitItemShare{With: []ItemShare{{"ali", 2}, {"sara", 1}}}},
		{"no units, no share", []string{"ali:0", "sara", "bob:1"}, 3, SplitItemShare{}},
		{"everyone by units too", []string{"ali:1", SplitEveryone}, 3, SplitItemShare{All: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSplitItemShare(tt.values, people, tt.qty)
			if got.All != tt.want.All || !slices.Equal(got.With, tt.want.With) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseChargeSplitMode(t *testing.T) {
	vat := &TxItem{Kind: TxItemVAT, Rate: ptr(14)}
	tip := &TxItem{Kind: TxItemTip, SplitMode: ChargeSplitByShare}
	tests := []struct {
		it   *TxItem
		v    string
		want ChargeSplitMode
	}{
		{vat, "share", ChargeSplitDefault},
		{vat, "equal", ChargeSplitEqually},
		{tip, "equal", ChargeSplitDefault},
		{tip, "share", ChargeSplitByShare},
		{tip, "", ChargeSplitByShare}, // not sent: kept
	}
	for _, tt := range tests {
		if got := ParseChargeSplitMode(tt.it, tt.v); got != tt.want {
			t.Errorf("%s %q = %v, want %v", tt.it.Label(), tt.v, got, tt.want)
		}
	}
}
