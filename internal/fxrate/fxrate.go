package fxrate

// NOTE: Check this
// https://openexchangerates.org/

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
)

type rate struct {
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

var mu sync.RWMutex

type fxRates map[string]rate

var rates fxRates = make(fxRates)

func init() {
	cacheDir := config.GetCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		logger.Get().Error("failed to create cache directory", zap.Error(err))
	}
}

func loadFromCache(currency string) (*rate, error) {
	l := logger.Get()

	cacheDir := config.GetCacheDir()
	filePath := filepath.Join(cacheDir, fmt.Sprintf("%s.cache", currency))
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			l.Debug("cache file didn't exist")
			return nil, nil
		}
		l.Warn("unknown error on retrieving the cache file", zap.Error(err))
		return nil, err
	}

	var cachedRate rate
	if err := json.Unmarshal(data, &cachedRate); err != nil {
		l.Error("error while unmarshaling JSON fx rate cache file", zap.Error(err))
		return nil, err
	}

	cacheDate, err := time.Parse(time.DateOnly, cachedRate.Date)
	if err != nil {
		l.Error("error while parsing fx rate cache date", zap.Error(err))
		return nil, err
	}

	expiresAt := cacheDate.Add(time.Hour * 24 * time.Duration(config.GetCacheExpiryInDays()))
	if time.Now().After(expiresAt) {
		l.Debug("expired cache removing the file...", zap.Time("cached_date", cacheDate))
		if err := os.Remove(filePath); err != nil {
			l.Warn("failed to remove stale cache", zap.String("currency", currency), zap.Error(err))
		}
		return nil, nil
	}

	return &cachedRate, nil
}

func saveToCache(currency string, r *rate) error {
	cacheDir := config.GetCacheDir()
	filePath := filepath.Join(cacheDir, fmt.Sprintf("%s.cache", currency))
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0o644)
}

func fetchFXRates(currency string) error {
	l := logger.Get()

	cachedRate, err := loadFromCache(currency)
	if err != nil {
		l.Warn("failed to load from cache", zap.String("currency", currency), zap.Error(err))
	} else if cachedRate != nil {
		mu.Lock()
		rates[currency] = *cachedRate
		mu.Unlock()
		return nil
	}

	url := fmt.Sprintf("https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/%s.min.json", currency)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var parsed map[string]any
	if err = json.Unmarshal(body, &parsed); err != nil {
		return err
	}

	_rates := make(map[string]float64, len(parsed[currency].(map[string]any)))

	for k, v := range parsed[currency].(map[string]any) {
		_rates[k] = float64(v.(float64))
	}

	newRate := rate{
		Date:  parsed["date"].(string),
		Rates: _rates,
	}

	if err := saveToCache(currency, &newRate); err != nil {
		l.Warn("failed to save to cache", zap.String("currency", currency), zap.Error(err))
	}

	mu.Lock()
	defer mu.Unlock()
	rates[currency] = newRate

	return nil
}

func GetFxRate(from, to string) (float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	if from == to {
		return 1, nil
	}

	mu.RLock()
	r, ok := rates[from]
	mu.RUnlock()

	if !ok {
		now := time.Now()

		if err := fetchFXRates(from); err != nil {
			return 0, err
		}

		l := logger.Get()
		l.Debug("retrieved fx rates", zap.Duration("took", time.Since(now)), zap.String("currency", from))

		mu.RLock()
		r = rates[from]
		mu.RUnlock()
	}

	return r.Rates[to], nil
}
