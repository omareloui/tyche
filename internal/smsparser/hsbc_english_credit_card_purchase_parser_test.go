package smsparser

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestHSBCEnglishCreditCardPurchaseParser_CanParse(t *testing.T) {
	parser := &HSBCEnglishCreditCardPurchaseParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC credit card purchase SMS",
			sms:      "Your Credit Card ending with *** 5450 has been used for AED 30.00 on 31/10/2025 at DU ONE-TIME PAY. Your available limit is EGP 29613.63",
			expected: true,
		},
		{
			name:     "invalid SMS - different prefix",
			sms:      "Your Debit Card ending with *** 5450 has been used for AED 30.00",
			expected: false,
		},
		{
			name:     "invalid SMS - missing 'has been used for'",
			sms:      "Your Credit Card ending with *** 5450 was charged AED 30.00",
			expected: false,
		},
		{
			name:     "invalid SMS - completely different format",
			sms:      "Transfer successful. Amount: 100 EGP",
			expected: false,
		},
		{
			name:     "empty SMS",
			sms:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.CanParse(tt.sms)
			if result != tt.expected {
				t.Errorf("CanParse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHSBCEnglishCreditCardPurchaseParser_Parse(t *testing.T) {
	parser := &HSBCEnglishCreditCardPurchaseParser{}

	tests := []struct {
		name    string
		sms     string
		want    *SMS
		wantErr bool
	}{
		{
			name: "valid SMS with single word counterparty",
			sms:  "Your Credit Card ending with *** 5450 has been used for AED 30.00 on 31/10/2025 at AMAZON. Your available limit is EGP 29613.63",
			want: &SMS{
				Bank:                     HSBC,
				TxKind:                   models.ExpenseTxKind,
				AccountID:                "5450",
				Currency:                 "AED",
				Amount:                   30.00,
				Timestamp:                time.Date(2025, 10, 31, 0, 0, 0, 0, time.UTC),
				Counterparty:             "AMAZON",
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         29613.63,
			},
			wantErr: false,
		},
		{
			name: "valid SMS with multi-word counterparty",
			sms:  "Your Credit Card ending with *** 5450 has been used for AED 30.00 on 31/10/2025 at DU ONE-TIME PAY. Your available limit is EGP 29613.63",
			want: &SMS{
				Bank:                     HSBC,
				TxKind:                   models.ExpenseTxKind,
				AccountID:                "5450",
				Currency:                 "AED",
				Amount:                   30.00,
				Timestamp:                time.Date(2025, 10, 31, 0, 0, 0, 0, time.UTC),
				Counterparty:             "DU ONE-TIME PAY",
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         29613.63,
			},
			wantErr: false,
		},
		{
			name: "valid SMS with different currencies",
			sms:  "Your Credit Card ending with *** 1234 has been used for USD 150.50 on 15/12/2024 at STARBUCKS COFFEE. Your available limit is EGP 50000.00",
			want: &SMS{
				Bank:                     HSBC,
				TxKind:                   models.ExpenseTxKind,
				AccountID:                "1234",
				Currency:                 "USD",
				Amount:                   150.50,
				Timestamp:                time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC),
				Counterparty:             "STARBUCKS COFFEE",
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         50000.00,
			},
			wantErr: false,
		},
		{
			name: "valid SMS with large amount",
			sms:  "Your Credit Card ending with *** 9876 has been used for EGP 12345.67 on 01/01/2025 at ELECTRONICS STORE. Your available limit is EGP 87654.33",
			want: &SMS{
				Bank:                     HSBC,
				TxKind:                   models.ExpenseTxKind,
				AccountID:                "9876",
				Currency:                 "EGP",
				Amount:                   12345.67,
				Timestamp:                time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Counterparty:             "ELECTRONICS STORE",
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         87654.33,
			},
			wantErr: false,
		},
		{
			name:    "invalid SMS - too short",
			sms:     "Your Credit Card",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - malformed",
			sms:     "Your Credit Card ending with *** 5450",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(tt.sms)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.Bank != tt.want.Bank {
				t.Errorf("Bank = %v, want %v", got.Bank, tt.want.Bank)
			}
			if got.TxKind != tt.want.TxKind {
				t.Errorf("TxKind = %v, want %v", got.TxKind, tt.want.TxKind)
			}
			if got.AccountID != tt.want.AccountID {
				t.Errorf("AccountID = %v, want %v", got.AccountID, tt.want.AccountID)
			}
			if got.Currency != tt.want.Currency {
				t.Errorf("Currency = %v, want %v", got.Currency, tt.want.Currency)
			}
			if got.Amount != tt.want.Amount {
				t.Errorf("Amount = %v, want %v", got.Amount, tt.want.Amount)
			}
			if !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
			}
			if got.Counterparty != tt.want.Counterparty {
				t.Errorf("Counterparty = %v, want %v", got.Counterparty, tt.want.Counterparty)
			}
			if got.AvailableBalanceCurrency != tt.want.AvailableBalanceCurrency {
				t.Errorf("AvailableBalanceCurrency = %v, want %v", got.AvailableBalanceCurrency, tt.want.AvailableBalanceCurrency)
			}
			if got.AvailableBalance != tt.want.AvailableBalance {
				t.Errorf("AvailableBalance = %v, want %v", got.AvailableBalance, tt.want.AvailableBalance)
			}
		})
	}
}
