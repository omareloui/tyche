package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishSalaryParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishSalaryParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC salary SMS",
			sms:      "From HSBC: 22JUL25 Salary to 123-456***-789 USD 5,000.00+ Your available balance is USD 5,000.00",
			expected: true,
		},
		{
			name:     "valid HSBC salary SMS with accumulated balance",
			sms:      "From HSBC: 27JUL25 Salary to 123-456***-789 USD 4,800.00+ Your available balance is USD 9,800.00",
			expected: true,
		},
		{
			name:     "invalid SMS - different prefix",
			sms:      "Your HSBC: 22JUL25 Salary to 123-456***-789 USD 5,000.00+",
			expected: false,
		},
		{
			name:     "invalid SMS - missing 'Salary to'",
			sms:      "From HSBC: 22JUL25 Deposit to 123-456***-789 USD 5,000.00+",
			expected: false,
		},
		{
			name:     "invalid SMS - purchase instead of salary",
			sms:      "From HSBC: 22JUL25 Purchase from 123-456***-789 USD 100.00-",
			expected: false,
		},
		{
			name:     "invalid SMS - completely different format",
			sms:      "Salary deposited: 5000 USD",
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

func TestHSBCEnglishSalaryParser_Parse(t *testing.T) {
	parser := &smsparser.HSBCEnglishSalaryParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "valid SMS - first salary of month",
			sms:  "From HSBC: 22JUL25 Salary to 123-456***-789 USD 5,000.00+ Your available balance is USD 5,000.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "USD",
				Amount:                   5000.00,
				Timestamp:                time.Date(2025, 7, 22, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "USD",
				AvailableBalance:         5000.00,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - salary added to existing balance",
			sms:  "From HSBC: 27JUL25 Salary to 123-456***-789 USD 4,800.00+ Your available balance is USD 9,800.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "USD",
				Amount:                   4800.00,
				Timestamp:                time.Date(2025, 7, 27, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "USD",
				AvailableBalance:         9800.00,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - September salary",
			sms:  "From HSBC: 24SEP25 Salary to 123-456***-789 USD 5,200.50+ Your available balance is USD 8,750.25",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "USD",
				Amount:                   5200.50,
				Timestamp:                time.Date(2025, 9, 24, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "USD",
				AvailableBalance:         8750.25,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - October salary",
			sms:  "From HSBC: 23OCT25 Salary to 123-456***-789 USD 4,950.75+ Your available balance is USD 12,500.80",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "USD",
				Amount:                   4950.75,
				Timestamp:                time.Date(2025, 10, 23, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "USD",
				AvailableBalance:         12500.80,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - different account number",
			sms:  "From HSBC: 15AUG25 Salary to 987-654***-321 EUR 6,500.00+ Your available balance is EUR 15,000.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "987-654***-321",
				Currency:                 "EUR",
				Amount:                   6500.00,
				Timestamp:                time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "EUR",
				AvailableBalance:         15000.00,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - high salary amount",
			sms:  "From HSBC: 01DEC25 Salary to 111-222***-333 GBP 12,500.99+ Your available balance is GBP 20,000.50",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "111-222***-333",
				Currency:                 "GBP",
				Amount:                   12500.99,
				Timestamp:                time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "GBP",
				AvailableBalance:         20000.50,
			},
			wantErr: false,
		},
		{
			name: "valid SMS - no decimals",
			sms:  "From HSBC: 10NOV25 Salary to 444-555***-666 USD 8,000+ Your available balance is USD 18,000",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.IncomeTxKind,
				AccountID:                "444-555***-666",
				Currency:                 "USD",
				Amount:                   8000.00,
				Timestamp:                time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
				AvailableBalanceCurrency: "USD",
				AvailableBalance:         18000.00,
			},
			wantErr: false,
		},
		{
			name:    "invalid SMS - too short",
			sms:     "From HSBC: 22JUL25",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - malformed",
			sms:     "From HSBC: 22JUL25 Salary to",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - wrong format",
			sms:     "Different bank salary message",
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
			if got.AvailableBalanceCurrency != tt.want.AvailableBalanceCurrency {
				t.Errorf("AvailableBalanceCurrency = %v, want %v", got.AvailableBalanceCurrency, tt.want.AvailableBalanceCurrency)
			}
			if got.AvailableBalance != tt.want.AvailableBalance {
				t.Errorf("AvailableBalance = %v, want %v", got.AvailableBalance, tt.want.AvailableBalance)
			}

			// Verify fields that should be empty for salary deposits
			if got.Counterparty != "" {
				t.Errorf("Counterparty = %q, want empty string", got.Counterparty)
			}
			if got.Ref != "" {
				t.Errorf("Ref = %q, want empty string", got.Ref)
			}
			if got.ToAmount != 0 {
				t.Errorf("ToAmount = %v, want 0", got.ToAmount)
			}
			if got.ToAccountID != "" {
				t.Errorf("ToAccountID = %q, want empty string", got.ToAccountID)
			}
			if got.ToCurrency != "" {
				t.Errorf("ToCurrency = %q, want empty string", got.ToCurrency)
			}
		})
	}
}

func TestHSBCEnglishSalaryParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.HSBCEnglishSalaryParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "valid SMS should not error",
			sms:     "From HSBC: 22JUL25 Salary to 123-456***-789 USD 5,000.00+ Your available balance is USD 5,000.00",
			wantErr: false,
		},
		{
			name:    "invalid date format",
			sms:     "From HSBC: 99ZZZ99 Salary to 123-456***-789 USD 5,000.00+ Your available balance is USD 5,000.00",
			wantErr: false, // Parser continues even with date parse error
		},
		{
			name:    "too few fields",
			sms:     "From HSBC: 22JUL25 Salary",
			wantErr: true,
		},
		{
			name:    "empty SMS",
			sms:     "",
			wantErr: true,
		},
		{
			name:    "wrong format - not salary",
			sms:     "From HSBC: 22JUL25 Purchase from 123-456***-789 USD 100.00-",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.sms)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
