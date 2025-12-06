package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishATMWithdrawalParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMWithdrawalParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC ATM withdrawal",
			sms:      "From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00- Your available balance is EGP 27,905.35",
			expected: true,
		},
		{
			name:     "HSBC ATM withdrawal with different amount",
			sms:      "From HSBC: 15MAR25 ATM Cash Withdrawal from 987-654***-321 EGP 500.00- Your available balance is EGP 10,000.00",
			expected: true,
		},
		{
			name:     "HSBC reversal message should not parse",
			sms:      "From HSBC: 28JUL25 ATM Cash Withdrawal Reversal from 123-456***-789 EGP 1,000.00",
			expected: false,
		},
		{
			name:     "non-HSBC message",
			sms:      "From OTHER: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00",
			expected: false,
		},
		{
			name:     "HSBC but not ATM withdrawal",
			sms:      "From HSBC: 28JUL25 Purchase at Store ABC EGP 500.00",
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

func TestHSBCEnglishATMWithdrawalParser_Parse(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMWithdrawalParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "valid ATM withdrawal",
			sms:  "From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00- Your available balance is EGP 27,905.35",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "EGP",
				Amount:                   1000.00,
				ToAccountID:              "Cash",
				ToCurrency:               "EGP",
				ToAmount:                 1000.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         27905.35,
				Timestamp:                time.Date(2025, 7, 28, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "different amount and balance",
			sms:  "From HSBC: 15MAR25 ATM Cash Withdrawal from 987-654***-321 EGP 500.50- Your available balance is EGP 10,250.75",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "987-654***-321",
				Currency:                 "EGP",
				Amount:                   500.50,
				ToAccountID:              "Cash",
				ToCurrency:               "EGP",
				ToAmount:                 500.50,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         10250.75,
				Timestamp:                time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name: "large amount with commas",
			sms:  "From HSBC: 01JAN25 ATM Cash Withdrawal from 111-222***-333 EGP 50,000.00- Your available balance is EGP 100,000.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "111-222***-333",
				Currency:                 "EGP",
				Amount:                   50000.00,
				ToAccountID:              "Cash",
				ToCurrency:               "EGP",
				ToAmount:                 50000.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         100000.00,
				Timestamp:                time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name:    "invalid message - reversal",
			sms:     "From HSBC: 28JUL25 ATM Cash Withdrawal Reversal from 123-456***-789 EGP 1,000.00",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid message - not HSBC",
			sms:     "From OTHER: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty string",
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
			if got.Currency != tt.want.Currency {
				t.Errorf("Currency = %v, want %v", got.Currency, tt.want.Currency)
			}
			if got.Amount != tt.want.Amount {
				t.Errorf("Amount = %v, want %v", got.Amount, tt.want.Amount)
			}
			if got.ToAccountID != tt.want.ToAccountID {
				t.Errorf("ToAccountID = %v, want %v", got.ToAccountID, tt.want.ToAccountID)
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

func TestHSBCEnglishATMWithdrawalParser_ParseError(t *testing.T) {
	parser := &smsparser.HSBCEnglishATMWithdrawalParser{}

	t.Run("returns ErrCantParse for invalid message", func(t *testing.T) {
		sms := "Invalid message"
		_, err := parser.Parse(sms)

		if err != smsparser.ErrCantParse {
			t.Errorf("Parse() error = %v, want %v", err, smsparser.ErrCantParse)
		}
	})
}

