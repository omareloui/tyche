// Package avatar downloads the users' profile pictures, kept in their
// database and served from /avatar as the provider's urls are rate limited.
package avatar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// MaxSize is the biggest picture downloaded.
const MaxSize = 256 << 10

var (
	ErrTooBig    = errors.New("picture too big")
	ErrNotImage  = errors.New("not an image")
	ErrBadStatus = errors.New("unexpected status")
)

type Picture struct {
	Data        []byte
	ContentType string
	// Hash versions the picture's url.
	Hash string
}

var client = &http.Client{Timeout: 5 * time.Second}

// Fetch downloads the picture at url.
func Fetch(ctx context.Context, url string) (*Picture, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrBadStatus, resp.StatusCode)
	}
	typ, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(typ, "image/") {
		return nil, ErrNotImage
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSize {
		return nil, ErrTooBig
	}

	sum := sha256.Sum256(data)
	return &Picture{Data: data, ContentType: typ, Hash: hex.EncodeToString(sum[:8])}, nil
}
