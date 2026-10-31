package handler

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

func TestMapFDToPurchase(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP", Symbol: "E£"}

	tests := []struct {
		name      string
		form      url.Values
		wantN     int
		wantPer   float64
		wantErr   error
		wantTotal float64
	}{
		{name: "cash", form: url.Values{"amount": {"E£ 1,000"}, "mode": {views.PayCash}, "installments": {"4"}}, wantTotal: 1000},
		{name: "months only", form: url.Values{"amount": {"1,200"}, "installments": {"12"}}, wantN: 12, wantPer: 100, wantTotal: 1200},
		{name: "per month only, even", form: url.Values{"amount": {"1,200"}, "installment_amount": {"E£ 300"}}, wantN: 4, wantPer: 300, wantTotal: 1200},
		{name: "per month only, uneven", form: url.Values{"amount": {"1,000"}, "installment_amount": {"300"}}, wantN: 4, wantPer: 250, wantTotal: 1000},
		{name: "both, with markup", form: url.Values{"amount": {"12,000"}, "installments": {"12"}, "installment_amount": {"1,100"}}, wantN: 12, wantPer: 1100, wantTotal: 13200},
		{name: "neither", form: url.Values{"amount": {"1,000"}}, wantErr: errNoInstallments, wantTotal: 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.form.Get("mode") == "" {
				tt.form.Set("mode", views.PayInstallments)
			}
			tt.form.Set("currency_id", "egp")
			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			p, err := mapFDToPurchase(r, []models.Currency{egp}, &egp)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if p.Installments != tt.wantN || p.InstallmentAmount != tt.wantPer || p.TotalCost() != tt.wantTotal {
				t.Errorf("got %d × %v = %v, want %d × %v = %v", p.Installments, p.InstallmentAmount, p.TotalCost(), tt.wantN, tt.wantPer, tt.wantTotal)
			}
		})
	}
}
