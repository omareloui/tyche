package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateCountsSearches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"candidates": [{"content": {"parts": [{"text": "{}"}]}, "finishReason": "STOP",
				"groundingMetadata": {"webSearchQueries": ["rent cairo", "rent berlin"],
					"groundingChunks": [{"web": {"uri": "https://example.com", "title": "Example"}}]}}],
			"usageMetadata": {"promptTokenCount": 1000, "candidatesTokenCount": 100, "thoughtsTokenCount": 100}
		}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "key", ModelID: "m", Endpoint: srv.URL}

	ans, usage, err := c.GenerateWith(context.Background(), []Part{{Text: "hi"}}, map[string]any{}, Options{Search: true})
	if err != nil {
		t.Fatal(err)
	}
	if usage != (Usage{InputTokens: 1000, OutputTokens: 200, SearchQueries: 2}) || len(ans.Sources) != 1 {
		t.Errorf("usage = %+v, sources = %v", usage, ans.Sources)
	}
	// 1000 × 0.75 + 200 × 3.75 + 2 searches at $14 / 1000 (14000 micros each).
	if got := usage.CostMicros(Prices{InputPerM: 0.75, OutputPerM: 3.75, SearchPerK: 14}); got != 1500+28000 {
		t.Errorf("cost = %d", got)
	}
	if !(Usage{SearchQueries: 1}).Billed() || (Usage{}).Billed() {
		t.Error("billed")
	}
}
