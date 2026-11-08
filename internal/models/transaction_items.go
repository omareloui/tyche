package models

import (
	"fmt"
	"math"
	"strings"

	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/idgen"
)

// TxItemKind is what a line of a transaction is: something bought, or a
// charge on top of (or inside) the items' prices.
type TxItemKind uint8

const (
	TxItemProduct TxItemKind = iota
	TxItemVAT
	TxItemService
	TxItemTip
	TxItemDelivery
	TxItemFee
	TxItemDiscount
	TxItemRounding
)

var txItemKindLabels = map[TxItemKind]string{
	TxItemProduct:  "Item",
	TxItemVAT:      "VAT",
	TxItemService:  "Service",
	TxItemTip:      "Tip",
	TxItemDelivery: "Delivery",
	TxItemFee:      "Fee",
	TxItemDiscount: "Discount",
	TxItemRounding: "Rounding",
}

func (k TxItemKind) String() string {
	if l, ok := txItemKindLabels[k]; ok {
		return l
	}
	return txItemKindLabels[TxItemProduct]
}

// IsCharge tells it's a charge (VAT, service, discount, ...), not an item.
func (k TxItemKind) IsCharge() bool { return k != TxItemProduct }

// TxItemChargeKinds are the charges, in the order they're offered.
func TxItemChargeKinds() []TxItemKind {
	return []TxItemKind{TxItemVAT, TxItemService, TxItemTip, TxItemDelivery, TxItemFee, TxItemDiscount, TxItemRounding}
}

// TxItemKindOf is the kind of a stored value, an item when unknown.
func TxItemKindOf(v int64) TxItemKind {
	k := TxItemKind(v)
	if _, ok := txItemKindLabels[k]; !ok {
		return TxItemProduct
	}
	return k
}

// TxItem is a line of a transaction (or of a recurrent transaction, copied to
// its occurrences): an item bought, or a charge. Amounts are in its
// transaction's account currency.
type TxItem struct {
	ID   string
	Kind TxItemKind `validate:"lte=7"`
	// Name is required for items; a charge without one is named after its
	// kind (Label).
	Name      string  `validate:"required_if=Kind 0,max=200" conform:"trim"`
	Quantity  float64 `validate:"gte=0"`
	UnitPrice float64
	// Total is the line's amount, negative for discounts.
	Total float64
	// Rate is the charge's percentage when the receipt shows one, e.g. 14.
	Rate *float64 `validate:"omitempty,gte=0,lte=100"`
	// Included tells a charge is already inside the items' prices ("VAT
	// included"): shown, never summed.
	Included bool
}

// Label is its name, else its kind's with the rate (e.g. "VAT 14%").
func (it *TxItem) Label() string {
	if it.Name != "" {
		return it.Name
	}
	if it.Rate != nil {
		return fmt.Sprintf("%s %s%%", it.Kind, formatters.FormatNum(*it.Rate))
	}
	return it.Kind.String()
}

// NormalizeTxItems fills what's missing of the items: a quantity of 1, a total of
// quantity × unit price or a unit price of total / quantity, a charge's total
// from its rate on the items' subtotal, a discount's total negative. Amounts
// are rounded with cur (cents when nil).
func NormalizeTxItems(items []TxItem, cur *Currency) {
	round := func(v float64) float64 {
		if cur != nil {
			return cur.Round(v)
		}
		return math.Round(v*100) / 100
	}
	for i := range items {
		it := &items[i]
		if it.Kind.IsCharge() {
			continue
		}
		if it.Quantity == 0 {
			it.Quantity = 1
		}
		switch {
		case it.Total == 0 && it.UnitPrice != 0:
			it.Total = round(it.Quantity * it.UnitPrice)
		case it.UnitPrice == 0 && it.Total != 0:
			it.UnitPrice = round(it.Total / it.Quantity)
		}
		it.Total = round(it.Total)
		it.UnitPrice = round(it.UnitPrice)
	}
	sub := TxItemsSubtotal(items)
	for i := range items {
		it := &items[i]
		if !it.Kind.IsCharge() {
			continue
		}
		it.Quantity, it.UnitPrice = 1, 0
		if it.Total == 0 && it.Rate != nil {
			it.Total = sub * *it.Rate / 100
		}
		if it.Kind == TxItemDiscount && it.Total > 0 {
			it.Total = -it.Total
		}
		it.Total = round(it.Total)
		it.UnitPrice = it.Total
	}
}

// TxItemsSubtotal is the items' total, charges left out.
func TxItemsSubtotal(items []TxItem) float64 {
	var sum float64
	for _, it := range items {
		if !it.Kind.IsCharge() {
			sum += it.Total
		}
	}
	return sum
}

// TxItemsTotal is what the lines add up to: the items plus the charges not
// already included in their prices.
func TxItemsTotal(items []TxItem) float64 {
	sum := TxItemsSubtotal(items)
	for _, it := range items {
		if it.Kind.IsCharge() && !it.Included {
			sum += it.Total
		}
	}
	return sum
}

// HasTxItemCharges tells some lines are charges.
func HasTxItemCharges(items []TxItem) bool {
	for _, it := range items {
		if it.Kind.IsCharge() {
			return true
		}
	}
	return false
}

// TxItemsMismatch tells the lines add up to something else than amount (by
// more than a cent), the items then being incomplete or wrong.
func TxItemsMismatch(items []TxItem, amount float64) bool {
	if len(items) == 0 {
		return false
	}
	return math.Abs(TxItemsTotal(items)-math.Abs(amount)) > 0.01
}

// CopyTxItems copies the items with new IDs, e.g. a recurrent transaction's
// onto an occurrence or a duplicated transaction's.
func CopyTxItems(items []TxItem) []TxItem {
	if len(items) == 0 {
		return nil
	}
	cp := make([]TxItem, len(items))
	for i, it := range items {
		it.ID = idgen.New()
		if it.Rate != nil {
			r := *it.Rate
			it.Rate = &r
		}
		cp[i] = it
	}
	return cp
}

// SortTxItems puts the items before the charges, each keeping its order.
func SortTxItems(items []TxItem) []TxItem {
	out := make([]TxItem, 0, len(items))
	for _, it := range items {
		if !it.Kind.IsCharge() {
			out = append(out, it)
		}
	}
	for _, it := range items {
		if it.Kind.IsCharge() {
			out = append(out, it)
		}
	}
	return out
}

// Receipt is a transaction's receipt image (the data served on its own).
type Receipt struct {
	ID          string
	ContentType string
	Hash        string
	// Size is its image's, in bytes.
	Size int64
}

// ViewSize is its image's size, e.g. "182 KB".
func (r *Receipt) ViewSize() string {
	return FormatBytes(r.Size)
}

// FormatBytes is a file size in KB (MB from 1 MB), e.g. "182 KB".
func FormatBytes(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1000*1024:
		return fmt.Sprintf("%d KB", max((n+512)/1024, 1))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// URL is where its image is served, cache busted by its version.
func (r *Receipt) URL() string {
	return "/receipts/" + r.ID + "?v=" + ReceiptVersion(r.Hash, r.ContentType)
}

// ReceiptVersion tells a receipt's image apart: its upload's hash, and its
// type, as converting a stored one (to WebP) keeps the hash.
func ReceiptVersion(hash, contentType string) string {
	return hash + "." + strings.TrimPrefix(contentType, "image/")
}
