package receiptocr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func fakeGemini(t *testing.T, status int, answer func(req map[string]any) string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "key" {
			t.Errorf("api key header = %q", r.Header.Get("x-goog-api-key"))
		}
		if !strings.HasSuffix(r.URL.Path, "/models/gemini-test:generateContent") {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer(req)))
	}))
	t.Cleanup(srv.Close)
	c := New("key", "gemini-test", "LOW")
	c.Endpoint = srv.URL
	return c
}

func candidate(text string) string {
	b, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"parts": []any{map[string]any{"text": text}}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 1800, "candidatesTokenCount": 400, "thoughtsTokenCount": 100},
	})
	return string(b)
}

func TestRead(t *testing.T) {
	result := `{"is_receipt":true,"merchant":"Cilantro","date":"2026-11-07","time":"18:42","currency":"EGP",
		"subtotal":180,"total":195.2,
		"items":[{"name":"Latte","quantity":2,"unit_price":45,"total":90},{"name":"Muffin","quantity":1,"unit_price":90,"total":90},{"name":" ","quantity":1,"unit_price":0,"total":0}],
		"charges":[{"kind":"vat","name":"VAT","rate":14,"amount":25.2,"included":true},{"kind":"service","name":"","rate":12,"amount":21.6,"included":false},
			{"kind":"discount","name":"Promo","rate":null,"amount":-6.4,"included":false},{"kind":"weird","name":"Bag","rate":null,"amount":1,"included":false}]}`
	c := fakeGemini(t, http.StatusOK, func(req map[string]any) string {
		gen := req["generationConfig"].(map[string]any)
		if gen["responseMimeType"] != "application/json" || gen["responseJsonSchema"] == nil {
			t.Errorf("generation config = %v", gen)
		}
		if gen["thinkingConfig"].(map[string]any)["thinkingLevel"] != "LOW" {
			t.Errorf("thinking config = %v", gen["thinkingConfig"])
		}
		parts := req["contents"].([]any)[0].(map[string]any)["parts"].([]any)
		inline := parts[0].(map[string]any)["inlineData"].(map[string]any)
		if inline["mimeType"] != "image/jpeg" || inline["data"] != "aW1n" {
			t.Errorf("inline data = %v", inline)
		}
		return candidate(result)
	})

	res, usage, err := c.Read(context.Background(), []byte("img"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens != 1800 || usage.OutputTokens != 500 {
		t.Errorf("usage = %+v", usage)
	}
	// 1800 × 0.75 + 500 × 3.75 micro dollars.
	if got := usage.CostMicros(0.75, 3.75); got != 3225 {
		t.Errorf("cost = %d", got)
	}
	if res.Merchant != "Cilantro" || res.Currency != "EGP" || res.Total != 195.2 {
		t.Errorf("result = %+v", res)
	}

	items := res.TxItems()
	if len(items) != 6 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Name != "Latte" || items[0].Quantity != 2 || items[1].Name != "Muffin" {
		t.Errorf("items = %+v", items[:2])
	}
	if items[2].Kind != models.TxItemVAT || !items[2].Included || *items[2].Rate != 14 {
		t.Errorf("vat = %+v", items[2])
	}
	if items[3].Kind != models.TxItemService || items[4].Kind != models.TxItemDiscount || items[4].Total != -6.4 {
		t.Errorf("charges = %+v", items[3:5])
	}
	if items[5].Kind != models.TxItemFee {
		t.Errorf("an unknown charge kind = %v, want a fee", items[5].Kind)
	}
	models.NormalizeTxItems(items, nil)
	if got := models.TxItemsTotal(items); got != 195.2+1 {
		t.Errorf("lines total = %v", got)
	}

	cairo, _ := time.LoadLocation("Africa/Cairo")
	when := res.When(cairo)
	if when.Format("2006-01-02 15:04") != "2026-11-07 18:42" || when.Location() != cairo {
		t.Errorf("when = %v", when)
	}
	res.Time = ""
	if got := res.When(cairo).Format("15:04"); got != "00:00" {
		t.Errorf("when without a time = %v", got)
	}
	res.Date = "07/11/2026"
	if !res.When(cairo).IsZero() {
		t.Error("expected a zero time for an unparsed date")
	}
}

func TestReadNotAReceipt(t *testing.T) {
	c := fakeGemini(t, http.StatusOK, func(map[string]any) string {
		return candidate(`{"is_receipt":false,"merchant":"","date":"","time":"","currency":"","subtotal":0,"total":0,"items":[],"charges":[]}`)
	})
	_, usage, err := c.Read(context.Background(), []byte("img"), "image/png")
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if usage.InputTokens == 0 {
		t.Error("expected the billed usage")
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string]struct {
		status     int
		body       string
		unreadable bool
	}{
		"api error":    {http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`, false},
		"not json":     {http.StatusBadGateway, `<html>bad gateway</html>`, false},
		"blocked":      {http.StatusOK, `{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":10}}`, true},
		"no candidate": {http.StatusOK, `{"candidates":[]}`, true},
		"bad answer":   {http.StatusOK, candidate(`{"is_receipt": tru`), true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := fakeGemini(t, tc.status, func(map[string]any) string { return tc.body })
			_, _, err := c.Read(context.Background(), []byte("img"), "image/jpeg")
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, ErrUnreadable) != tc.unreadable {
				t.Errorf("err = %v, unreadable want %v", err, tc.unreadable)
			}
			if name == "api error" && !strings.Contains(err.Error(), "quota exceeded") {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestNewWithoutKey(t *testing.T) {
	if New("", "m", "") != nil {
		t.Error("expected no client without an API key")
	}
}
