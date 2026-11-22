// Package gemini calls Gemini's generateContent over plain net/http, asking
// for JSON following a schema.
package gemini

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
)

// DefaultEndpoint is Gemini's API base.
const DefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta"

// ErrNoAnswer is a call answered without any text: blocked, or no candidate.
var ErrNoAnswer = errors.New("gemini gave no answer")

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

// Part is a piece of the prompt: text or inline data (e.g. an image).
type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// Inline is a part of the data, base64 encoded.
func Inline(mimeType string, data []byte) Part {
	return Part{InlineData: &InlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(data)}}
}

// Client calls a Gemini model.
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

// Answer is the text of the first candidate (the JSON asked for).
type Answer struct {
	Text         string
	FinishReason string
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []Part `json:"parts"`
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

// Generate asks the model for JSON following schema. The usage is returned
// even on errors when the call was billed.
func (c *Client) Generate(ctx context.Context, parts []Part, schema map[string]any) (*Answer, Usage, error) {
	gen := map[string]any{
		"responseMimeType":   "application/json",
		"responseJsonSchema": schema,
	}
	if c.ThinkingLevel != "" {
		gen["thinkingConfig"] = map[string]any{"thinkingLevel": c.ThinkingLevel}
	}
	body, err := json.Marshal(map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": parts}},
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
		return nil, usage, fmt.Errorf("%w (blocked: %s)", ErrNoAnswer, gr.PromptFeedback.BlockReason)
	}
	if len(gr.Candidates) == 0 {
		return nil, usage, ErrNoAnswer
	}
	var text strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	return &Answer{Text: text.String(), FinishReason: gr.Candidates[0].FinishReason}, usage, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
