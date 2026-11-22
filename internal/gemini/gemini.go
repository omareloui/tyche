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
	// SearchQueries are the Google Search queries a grounded call
	// (Options.Search) ran, billed apart from the tokens.
	SearchQueries int
}

// Add is the usage of both calls.
func (u Usage) Add(o Usage) Usage {
	return Usage{
		InputTokens:   u.InputTokens + o.InputTokens,
		OutputTokens:  u.OutputTokens + o.OutputTokens,
		SearchQueries: u.SearchQueries + o.SearchQueries,
	}
}

// Billed tells the call used anything billed.
func (u Usage) Billed() bool {
	return u.InputTokens > 0 || u.OutputTokens > 0 || u.SearchQueries > 0
}

// Prices are dollars per million tokens and per thousand search queries.
type Prices struct {
	InputPerM, OutputPerM, SearchPerK float64
}

// CostMicros is the usage's cost in millionths of a dollar at the prices.
func (u Usage) CostMicros(p Prices) int64 {
	return int64(math.Round(float64(u.InputTokens)*p.InputPerM + float64(u.OutputTokens)*p.OutputPerM + float64(u.SearchQueries)*p.SearchPerK*1000))
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
	// Sources are the web pages a grounded answer (Options.Search) used.
	Sources []Source
}

// Source is a web page an answer was grounded on.
type Source struct {
	Title string
	URL   string
}

// Options change a call.
type Options struct {
	// Search grounds the answer on Google Search.
	Search bool
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []Part `json:"parts"`
		} `json:"content"`
		FinishReason      string `json:"finishReason"`
		GroundingMetadata struct {
			WebSearchQueries []string `json:"webSearchQueries"`
			GroundingChunks  []struct {
				Web *struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
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
	return c.GenerateWith(ctx, parts, schema, Options{})
}

// GenerateWith is Generate with options.
func (c *Client) GenerateWith(ctx context.Context, parts []Part, schema map[string]any, opts Options) (*Answer, Usage, error) {
	gen := map[string]any{
		"responseMimeType":   "application/json",
		"responseJsonSchema": schema,
	}
	if c.ThinkingLevel != "" {
		gen["thinkingConfig"] = map[string]any{"thinkingLevel": c.ThinkingLevel}
	}
	payload := map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": parts}},
		"generationConfig": gen,
	}
	if opts.Search {
		payload["tools"] = []map[string]any{{"google_search": map[string]any{}}}
	}
	body, err := json.Marshal(payload)
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
	for _, c := range gr.Candidates {
		usage.SearchQueries += len(c.GroundingMetadata.WebSearchQueries)
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
	ans := &Answer{Text: text.String(), FinishReason: gr.Candidates[0].FinishReason}
	for _, ch := range gr.Candidates[0].GroundingMetadata.GroundingChunks {
		if ch.Web != nil && ch.Web.URI != "" {
			ans.Sources = append(ans.Sources, Source{Title: ch.Web.Title, URL: ch.Web.URI})
		}
	}
	return ans, usage, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
