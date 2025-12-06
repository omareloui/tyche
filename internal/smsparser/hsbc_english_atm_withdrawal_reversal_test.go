package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishATMReversalParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMReversalParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC ATM reversal",
			sms:      "From HSBC: 28JUL25 ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+ Your available balance is EGP 28,905.35",
			expected: true,
		},
		{
			name:     "different date and amount",
			sms:      "From HSBC: 01JAN25 ATM Reversal of Cash Withdrawal from 987-654***-321 EGP 500.50+ Your available balance is EGP 12,345.67",
			expected: true,
		},
		{
			name:     "withdrawal message should not parse",
			sms:      "From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00-",
			expected: false,
		},
		{
			name:     "non-HSBC message",
			sms:      "From OTHER: ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+",
			expected: false,
		},
		{
			name:     "HSBC but different transaction",
			sms:      "From HSBC: 12DEC25 Purchase at Store XYZ EGP 200.00",
			expected: false,
		},
		{
			name:     "empty string",
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

func TestHSBCEnglishATMReversalParser_Parse(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMReversalParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "valid reversal",
			sms:  "From HSBC: 28JUL25 ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+ Your available balance is EGP 28,905.35",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "Cash",
				ToAccountID:              "123-456***-789",
				ToCurrency:               "EGP",
				ToAmount:                 1000.00,
				Currency:                 "EGP",
				Amount:                   1000.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         28905.35,
				Timestamp:                time.Date(2025, 7, 28, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "different amount and balance",
			sms:  "From HSBC: 01JAN25 ATM Reversal of Cash Withdrawal from 987-654***-321 EGP 500.50+ Your available balance is EGP 12,345.67",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "Cash",
				ToAccountID:              "987-654***-321",
				ToCurrency:               "EGP",
				ToAmount:                 500.50,
				Currency:                 "EGP",
				Amount:                   500.50,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         12345.67,
				Timestamp:                time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "large amount with commas",
			sms:  "From HSBC: 15APR25 ATM Reversal of Cash Withdrawal from 111-222***-333 EGP 50,000.00+ Your available balance is EGP 100,000.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "Cash",
				ToAccountID:              "111-222***-333",
				ToCurrency:               "EGP",
				ToAmount:                 50000.00,
				Currency:                 "EGP",
				Amount:                   50000.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         100000.00,
				Timestamp:                time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name:    "withdrawal should not parse",
			sms:     "From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00-",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "not HSBC",
			sms:     "From OTHER: ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty message",
			sms:     "",
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
			if got.ToAccountID != tt.want.ToAccountID {
				t.Errorf("ToAccountID = %v, want %v", got.ToAccountID, tt.want.ToAccountID)
			}
			if got.Currency != tt.want.Currency {
				t.Errorf("Currency = %v, want %v", got.Currency, tt.want.Currency)
			}
			if got.Amount != tt.want.Amount {
				t.Errorf("Amount = %v, want %v", got.Amount, tt.want.Amount)
			}
			if got.ToCurrency != tt.want.ToCurrency {
				t.Errorf("ToCurrency = %v, want %v", got.ToCurrency, tt.want.ToCurrency)
			}
			if got.ToAmount != tt.want.ToAmount {
				t.Errorf("ToAmount = %v, want %v", got.ToAmount, tt.want.ToAmount)
			}
			if got.AvailableBalanceCurrency != tt.want.AvailableBalanceCurrency {
				t.Errorf("AvailableBalanceCurrency = %v, want %v", got.AvailableBalanceCurrency, tt.want.AvailableBalanceCurrency)
			}
			if got.AvailableBalance != tt.want.AvailableBalance {
				t.Errorf("AvailableBalance = %v, want %v", got.AvailableBalance, tt.want.AvailableBalance)
			}
			if !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
			}
		})
	}
}

func TestHSBCEnglishATMReversalParser_ParseError(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMReversalParser{}

	t.Run("returns ErrCantParse for invalid message", func(t *testing.T) {
		sms := "Invalid message"
		_, err := parser.Parse(sms)

		if err != smsparser.ErrCantParse {
			t.Errorf("Parse() error = %v, want %v", err, smsparser.ErrCantParse)
		}
	})
}
