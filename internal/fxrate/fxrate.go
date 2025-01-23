package fxrate

// NOTE: Check this
// https://openexchangerates.org/

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type rate struct {
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

type fxRates map[string]rate

var rates fxRates = make(fxRates)

func fetchFXRates(currency string) error {
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

	rates[currency] = rate{
		Date:  parsed["date"].(string),
		Rates: _rates,
	}

	return nil
}

func GetFxRate(from, to string) (float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	if from == to {
		return 1, nil
	}

	r, ok := rates[from]
	if !ok {
		if err := fetchFXRates(from); err != nil {
			return 0, err
		}
		r = rates[from]
	}
	return r.Rates[to], nil
}
