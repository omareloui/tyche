// Package receiptocr reads receipts' images with Gemini: the merchant, date,
// currency, items and charges (VAT, service, discounts, ...).
package receiptocr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
)

// ErrUnreadable is an image that isn't a receipt or couldn't be read.
var ErrUnreadable = errors.New("couldn't read a receipt in the image")

// Reader reads a receipt's image (an interface so tests fake it).
type Reader interface {
	Read(ctx context.Context, img []byte, contentType string) (*Result, Usage, error)
	Model() string
}

// Item is a line bought.
type Item struct {
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Total     float64 `json:"total"`
	// WarrantyMonths is the item's own warranty when printed next to it, 0
	// when not.
	WarrantyMonths int `json:"warranty_months"`
}

// Charge is a line on top of (or inside) the items' prices.
type Charge struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Rate     *float64 `json:"rate"`
	Amount   float64  `json:"amount"`
	Included bool     `json:"included"`
}

// Result is what was read off a receipt; anything missing is zero.
type Result struct {
	IsReceipt bool     `json:"is_receipt"`
	Merchant  string   `json:"merchant"`
	Date      string   `json:"date"`
	Time      string   `json:"time"`
	Currency  string   `json:"currency"`
	Subtotal  float64  `json:"subtotal"`
	Total     float64  `json:"total"`
	Items     []Item   `json:"items"`
	Charges   []Charge `json:"charges"`
	// ReturnDays and WarrantyMonths are the receipt's return policy and
	// whole purchase's warranty when printed, 0 when not.
	ReturnDays     int `json:"return_days"`
	WarrantyMonths int `json:"warranty_months"`
}

// Usage is what a call used, as billed.
type Usage = gemini.Usage

var chargeKinds = map[string]models.TxItemKind{
	"vat":      models.TxItemVAT,
	"tax":      models.TxItemVAT,
	"service":  models.TxItemService,
	"tip":      models.TxItemTip,
	"delivery": models.TxItemDelivery,
	"fee":      models.TxItemFee,
	"discount": models.TxItemDiscount,
	"rounding": models.TxItemRounding,
}

// TxItems are the result's lines: the items, then the charges.
func (r *Result) TxItems() []models.TxItem {
	return r.TxItemsOn(time.Time{})
}

// TxItemsOn are TxItems of a purchase on day, the items' printed warranties
// counted from it (none when day is zero).
func (r *Result) TxItemsOn(day time.Time) []models.TxItem {
	items := []models.TxItem{}
	for _, it := range r.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		line := models.TxItem{Name: name, Quantity: it.Quantity, UnitPrice: it.UnitPrice, Total: it.Total}
		if !day.IsZero() {
			_, line.WarrantyUntil = models.WindowsFromReceipt(day, 0, it.WarrantyMonths)
		}
		items = append(items, line)
	}
	for _, c := range r.Charges {
		kind, ok := chargeKinds[strings.ToLower(strings.TrimSpace(c.Kind))]
		if !ok {
			kind = models.TxItemFee
		}
		if c.Amount == 0 && c.Rate == nil {
			continue
		}
		items = append(items, models.TxItem{
			Kind: kind, Name: strings.TrimSpace(c.Name), Total: c.Amount, Rate: c.Rate, Included: c.Included,
		})
	}
	return items
}

// When is the receipt's date (and time when read) in loc, zero when it has
// none or it doesn't parse.
func (r *Result) When(loc *time.Location) time.Time {
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(r.Date), loc)
	if err != nil {
		return time.Time{}
	}
	if t, err := time.Parse("15:04", strings.TrimSpace(r.Time)); err == nil {
		d = d.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
	}
	return d
}

// Client reads receipts with Gemini's generateContent.
type Client struct {
	*gemini.Client
}

// New is a client of the model, nil without an API key.
func New(apiKey, model, thinkingLevel string) *Client {
	c := gemini.New(apiKey, model, thinkingLevel)
	if c == nil {
		return nil
	}
	return &Client{Client: c}
}

const prompt = `You read photos of shop and restaurant receipts (Arabic, English or both) for a personal finance app.
Extract, as printed:
- merchant: the shop's name (the brand, not the legal company or branch address)
- date as YYYY-MM-DD and time as HH:MM (24h), empty when not printed
- currency: ISO 4217 code (EGP for "ج.م", "LE" or "EGP"), empty when unclear
- items: every product line with its name as printed, quantity (1 when not printed), unit price and line total
- charges: every line that isn't a product: VAT or other tax, service, tip, delivery, fees, discounts, rounding. kind is one of vat, tax, service, tip, delivery, fee, discount, rounding; rate is the percentage when printed (14 for 14%); amount is negative for discounts; included is true when the line says the charge is already included in the prices (e.g. "VAT included", "شامل الضريبة")
- subtotal and total: as printed, the total being what was paid
- return_days: the days the receipt says the purchase can be returned or exchanged within (14 for "returns within 14 days"), 0 when not printed
- warranty_months: the warranty the receipt prints for the whole purchase, in months (24 for "2 years warranty"), 0 when not printed; an item's own warranty printed next to it goes in that item's warranty_months (0 when none)
Don't invent lines or prices you can't read. Set is_receipt to false when the image isn't a receipt or bill.`

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"is_receipt":      map[string]any{"type": "boolean"},
		"merchant":        map[string]any{"type": "string"},
		"date":            map[string]any{"type": "string"},
		"time":            map[string]any{"type": "string"},
		"currency":        map[string]any{"type": "string"},
		"subtotal":        map[string]any{"type": "number"},
		"total":           map[string]any{"type": "number"},
		"return_days":     map[string]any{"type": "integer"},
		"warranty_months": map[string]any{"type": "integer"},
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":            map[string]any{"type": "string"},
					"quantity":        map[string]any{"type": "number"},
					"unit_price":      map[string]any{"type": "number"},
					"total":           map[string]any{"type": "number"},
					"warranty_months": map[string]any{"type": "integer"},
				},
				"required": []string{"name", "quantity", "unit_price", "total", "warranty_months"},
			},
		},
		"charges": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":     map[string]any{"type": "string", "enum": []string{"vat", "tax", "service", "tip", "delivery", "fee", "discount", "rounding"}},
					"name":     map[string]any{"type": "string"},
					"rate":     map[string]any{"type": []string{"number", "null"}},
					"amount":   map[string]any{"type": "number"},
					"included": map[string]any{"type": "boolean"},
				},
				"required": []string{"kind", "name", "amount", "included"},
			},
		},
	},
	"required": []string{"is_receipt", "merchant", "date", "time", "currency", "subtotal", "total", "items", "charges", "return_days", "warranty_months"},
}

// Read reads the receipt in img. The usage is returned even on errors when
// the call was billed.
func (c *Client) Read(ctx context.Context, img []byte, contentType string) (*Result, Usage, error) {
	ans, usage, err := c.Generate(ctx, []gemini.Part{gemini.Inline(contentType, img), {Text: prompt}}, responseSchema)
	if errors.Is(err, gemini.ErrNoAnswer) {
		return nil, usage, fmt.Errorf("%w (%v)", ErrUnreadable, err)
	}
	if err != nil {
		return nil, usage, err
	}
	var result Result
	if err := json.Unmarshal([]byte(ans.Text), &result); err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", ErrUnreadable, ans.FinishReason)
	}
	if !result.IsReceipt || (len(result.Items) == 0 && result.Total == 0) {
		return &result, usage, ErrUnreadable
	}
	return &result, usage, nil
}
