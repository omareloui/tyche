// Package receiptocr reads receipts' images with Gemini: the merchant, date,
// currency, items and charges (VAT, service, discounts, ...).
package receiptocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

// DefaultEndpoint is Gemini's API base.
const DefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta"

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
}

// Usage is what a call used, as billed.
type Usage struct {
	InputTokens int
	// OutputTokens include the thinking ones.
	OutputTokens int
}

// CostMicros is the usage's cost in millionths of a dollar at the prices,
// in dollars per million tokens.
func (u Usage) CostMicros(inputPerM, outputPerM float64) int64 {
	return int64(math.Round(float64(u.InputTokens)*inputPerM + float64(u.OutputTokens)*outputPerM))
}

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
	items := []models.TxItem{}
	for _, it := range r.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		items = append(items, models.TxItem{Name: name, Quantity: it.Quantity, UnitPrice: it.UnitPrice, Total: it.Total})
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
	APIKey string
	// ModelID is e.g. gemini-3.8-flash.
	ModelID string
	// ThinkingLevel (MINIMAL, LOW, ...), empty for the model's default.
	ThinkingLevel string
	// Endpoint is the API base, DefaultEndpoint when empty.
	Endpoint string
	HTTP     *http.Client
}

// New is a client of the model, nil without an API key.
func New(apiKey, model, thinkingLevel string) *Client {
	if apiKey == "" {
		return nil
	}
	return &Client{APIKey: apiKey, ModelID: model, ThinkingLevel: thinkingLevel, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) Model() string { return c.ModelID }

const prompt = `You read photos of shop and restaurant receipts (Arabic, English or both) for a personal finance app.
Extract, as printed:
- merchant: the shop's name (the brand, not the legal company or branch address)
- date as YYYY-MM-DD and time as HH:MM (24h), empty when not printed
- currency: ISO 4217 code (EGP for "ج.م", "LE" or "EGP"), empty when unclear
- items: every product line with its name as printed, quantity (1 when not printed), unit price and line total
- charges: every line that isn't a product: VAT or other tax, service, tip, delivery, fees, discounts, rounding. kind is one of vat, tax, service, tip, delivery, fee, discount, rounding; rate is the percentage when printed (14 for 14%); amount is negative for discounts; included is true when the line says the charge is already included in the prices (e.g. "VAT included", "شامل الضريبة")
- subtotal and total: as printed, the total being what was paid
Don't invent lines or prices you can't read. Set is_receipt to false when the image isn't a receipt or bill.`

var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"is_receipt": map[string]any{"type": "boolean"},
		"merchant":   map[string]any{"type": "string"},
		"date":       map[string]any{"type": "string"},
		"time":       map[string]any{"type": "string"},
		"currency":   map[string]any{"type": "string"},
		"subtotal":   map[string]any{"type": "number"},
		"total":      map[string]any{"type": "number"},
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string"},
					"quantity":   map[string]any{"type": "number"},
					"unit_price": map[string]any{"type": "number"},
					"total":      map[string]any{"type": "number"},
				},
				"required": []string{"name", "quantity", "unit_price", "total"},
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
	"required": []string{"is_receipt", "merchant", "date", "time", "currency", "subtotal", "total", "items", "charges"},
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Read reads the receipt in img. The usage is returned even on errors when
// the call was billed.
func (c *Client) Read(ctx context.Context, img []byte, contentType string) (*Result, Usage, error) {
	gen := map[string]any{
		"responseMimeType":   "application/json",
		"responseJsonSchema": responseSchema,
	}
	if c.ThinkingLevel != "" {
		gen["thinkingConfig"] = map[string]any{"thinkingLevel": c.ThinkingLevel}
	}
	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []part{
				{InlineData: &inlineData{MimeType: contentType, Data: base64.StdEncoding.EncodeToString(img)}},
				{Text: prompt},
			},
		}},
		"generationConfig": gen,
	})
	if err != nil {
		return nil, Usage{}, err
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/models/%s:generateContent", endpoint, c.ModelID), bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)

	httpc := c.HTTP
	if httpc == nil {
		httpc = http.DefaultClient
	}
	res, err := httpc.Do(req)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("calling gemini: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("reading gemini's answer: %w", err)
	}

	var gr generateResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, Usage{}, fmt.Errorf("gemini answered %d: %s", res.StatusCode, truncate(string(raw), 300))
	}
	usage := Usage{
		InputTokens:  gr.UsageMetadata.PromptTokenCount,
		OutputTokens: gr.UsageMetadata.CandidatesTokenCount + gr.UsageMetadata.ThoughtsTokenCount,
	}
	if res.StatusCode != http.StatusOK {
		msg := res.Status
		if gr.Error != nil && gr.Error.Message != "" {
			msg = gr.Error.Message
		}
		return nil, usage, fmt.Errorf("gemini answered %d: %s", res.StatusCode, msg)
	}
	if gr.PromptFeedback.BlockReason != "" {
		return nil, usage, fmt.Errorf("%w (blocked: %s)", ErrUnreadable, gr.PromptFeedback.BlockReason)
	}
	if len(gr.Candidates) == 0 {
		return nil, usage, ErrUnreadable
	}
	var text strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	var result Result
	if err := json.Unmarshal([]byte(text.String()), &result); err != nil {
		return nil, usage, fmt.Errorf("%w (finish reason %s)", ErrUnreadable, gr.Candidates[0].FinishReason)
	}
	if !result.IsReceipt || (len(result.Items) == 0 && result.Total == 0) {
		return &result, usage, ErrUnreadable
	}
	return &result, usage, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
