package fxrate

import (
	"encoding/json"
	"io"
	"net/http"
)

type usdFxRate struct {
	Date string             `json:"date"`
	Usd  map[string]float64 `json:"usd"`
}

var usdFxRates usdFxRate

func (fx *usdFxRate) To(cur string) float64 {
	return fx.Usd[cur]
}

func (fx *usdFxRate) ToEgp() float64 {
	return fx.To("egp")
}

func fetchFXRates() error {
	resp, err := http.Get("https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.min.json")
	if err != nil {
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, &usdFxRates)
}

func UsdToEgp() (float64, error) {
	if len(usdFxRates.Usd) == 0 {
		if err := fetchFXRates(); err != nil {
			return 0, err
		}
	}
	return usdFxRates.ToEgp(), nil
}
