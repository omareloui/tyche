package handler

import (
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
		name string
		form url.Values
		want *models.InstallmentPlan
	}{
		{name: "cash", form: url.Values{"mode": {views.PayCash}, "installments": {"4"}}},
		{
			name: "known amount drops the rate",
			form: url.Values{"strategy": {"known"}, "installments": {"12"}, "installment_amount": {"E£ 1,100"}, "rate": {"10"}},
			want: &models.InstallmentPlan{Months: 12, Amount: 1100},
		},
		{
			name: "flat monthly rate, extras",
			form: url.Values{
				"strategy": {"flat"}, "installments": {"6"}, "rate": {"1.5"}, "rate_period": {"month"}, "installment_amount": {"999"},
				"down_payment": {"20"}, "down_payment_unit": {"pct"},
				"admin_fee": {"E£ 250"}, "admin_fee_unit": {"amount"}, "admin_fee_financed": {"on"},
				"monthly_fee": {"E£ 15"}, "insurance_rate": {"1.2"}, "balloon_pct": {"10"}, "grace_months": {"2"},
			},
			want: &models.InstallmentPlan{
				Strategy: models.StrategyFlat, Months: 6, Rate: 1.5, RatePeriod: models.RatePerMonth,
				DownPayment: 20, DownPaymentPct: true, AdminFee: 250, AdminFeeFinanced: true,
				MonthlyFee: 15, InsuranceRate: 1.2, BalloonPct: 10, GraceMonths: 2,
			},
		},
		{
			name: "decreasing",
			form: url.Values{"strategy": {"decreasing"}, "installments": {"60"}, "rate": {"18"}},
			want: &models.InstallmentPlan{Strategy: models.StrategyDecreasing, Months: 60, Rate: 18},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.form.Get("mode") == "" {
				tt.form.Set("mode", views.PayInstallments)
			}
			tt.form.Set("amount", "E£ 12,000")
			tt.form.Set("currency_id", "egp")
			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			p, err := mapFDToPurchase(r, []models.Currency{egp}, &egp)
			if err != nil {
				t.Fatal(err)
			}
			if p.Price != 12000 || p.Currency.ID != "egp" {
				t.Errorf("purchase = %+v", p)
			}
			switch {
			case tt.want == nil && p.Plan != nil:
				t.Errorf("plan = %+v, want cash", p.Plan)
			case tt.want != nil && (p.Plan == nil || *p.Plan != *tt.want):
				t.Errorf("plan = %+v, want %+v", p.Plan, tt.want)
			}

			// What's read fills the form back the same.
			fd := mapPurchaseToFD(p, nil)
			if (p.Plan != nil) != (fd.Mode.Value == views.PayInstallments) {
				t.Errorf("mode = %q", fd.Mode.Value)
			}
		})
	}
}

func TestWishlistPlanTxs(t *testing.T) {
	egp := models.Currency{ID: "egp", String: "EGP", Symbol: "E£"}
	acc := models.Account{ID: "acc", Currency: egp}

	t.Run("down payment first", func(t *testing.T) {
		item := &models.WishlistItem{Name: "Car", Price: 1200, Currency: egp, Plan: &models.InstallmentPlan{Months: 3, DownPayment: 300}}
		first := &models.Transaction{Account: acc, Amount: 300, Kind: models.ExpenseTxKind}
		first.ScheduledTo = first.ScheduledTo.AddDate(2026, 0, 0)

		txs := wishlistPlanTxs(item, first)
		if len(txs) != 3 || txs[0].Amount != 300 || txs[0].Description != "Car (1/3)" || !txs[0].ScheduledTo.Equal(first.ScheduledTo.AddDate(0, 1, 0)) {
			t.Errorf("txs = %+v", txs)
		}
	})

	t.Run("first monthly payment first", func(t *testing.T) {
		item := &models.WishlistItem{Name: "Phone", Price: 1200, Currency: egp, Plan: &models.InstallmentPlan{Strategy: models.StrategyDecreasing, Months: 3, Rate: 12}}
		s := item.Schedule()
		first := &models.Transaction{Account: acc, Amount: s.FirstPayment(), Kind: models.ExpenseTxKind}
		first.ScheduledTo = first.ScheduledTo.AddDate(2026, 1, 0)

		txs := wishlistPlanTxs(item, first)
		if len(txs) != 2 || txs[0].Description != "Phone (2/3)" || txs[1].Amount != 404 || !txs[1].ScheduledTo.Equal(first.ScheduledTo.AddDate(0, 2, 0)) {
			t.Errorf("txs = %+v", txs)
		}
	})

	t.Run("cash", func(t *testing.T) {
		if txs := wishlistPlanTxs(&models.WishlistItem{Price: 10, Currency: egp}, &models.Transaction{Account: acc}); txs != nil {
			t.Errorf("txs = %+v", txs)
		}
	})
}
