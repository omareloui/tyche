package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishTransferParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishTransferParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid transfer",
			sms:      "From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85",
			expected: true,
		},
		{
			name:     "valid transfer with different date and amount",
			sms:      "From HSBC: 15JAN25 Transfer from 123-456***-789 EGP 1,250.50- Your available balance is EGP 20,500.75",
			expected: true,
		},
		{
			name:     "should not parse salary transfer",
			sms:      "From HSBC: 15JAN25 Salary Transfer from HR EGP 10,000.00",
			expected: false,
		},
		{
			name:     "should not parse IPN transfer",
			sms:      "From HSBC: 15JAN25 Transfer from 123-456***-789 IPN EGP 300.00",
			expected: false,
		},
		{
			name:     "non-HSBC prefix",
			sms:      "From OTHER: 07OCT25 Transfer from 123-456***-789 EGP 360.00",
			expected: false,
		},
		{
			name:     "HSBC but not transfer",
			sms:      "From HSBC: 07OCT25 Purchase at ABC Store EGP 200.00",
			expected: false,
		},
		{
			name:     "empty message",
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

func TestHSBCEnglishTransferParser_Parse(t *testing.T) {
	parser := &smsparser.HSBCEnglishTransferParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "valid transfer",
			sms:  "From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "123-456***-789",
				Currency:                 "EGP",
				Amount:                   360.00,
				ToCurrency:               "EGP",
				ToAmount:                 360.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         49647.85,
				Timestamp:                time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC),
				Counterparty:             "Transfer",
			},
			wantErr: false,
		},
		{
			name: "large transfer with commas",
			sms:  "From HSBC: 01FEB25 Transfer from 999-888***-777 EGP 50,000.00- Your available balance is EGP 150,000.00",
			want: &smsparser.SMS{
				Bank:                     smsparser.HSBC,
				TxKind:                   models.TransferTxKind,
				AccountID:                "999-888***-777",
				Currency:                 "EGP",
				Amount:                   50000.00,
				ToCurrency:               "EGP",
				ToAmount:                 50000.00,
				AvailableBalanceCurrency: "EGP",
				AvailableBalance:         150000.00,
				Timestamp:                time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
				Counterparty:             "Transfer",
			},
			wantErr: false,
		},
		{
			name: "invalid salary transfer",
			sms:  "From HSBC: 07OCT25 Salary Transfer from HR EGP 360.00",
			want: nil, wantErr: true,
		},
		{
			name: "invalid IPN transfer",
			sms:  "From HSBC: 07OCT25 Transfer from 123-456***-789 IPN EGP 360.00",
			want: nil, wantErr: true,
		},
		{
			name: "too short message",
			sms:  "From HSBC: 07OCT25 Transfer from 123-456***-789",
			want: nil, wantErr: true,
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
			if got.Counterparty != tt.want.Counterparty {
				t.Errorf("Counterparty = %v, want %v", got.Counterparty, tt.want.Counterparty)
			}
		})
	}
}

func TestHSBCEnglishTransferParser_ParseError(t *testing.T) {
	parser := &smsparser.HSBCEnglishTransferParser{}

	t.Run("returns ErrCantParse for invalid message", func(t *testing.T) {
		sms := "Invalid message"
		_, err := parser.Parse(sms)

		if err != smsparser.ErrCantParse {
			t.Errorf("Parse() error = %v, want %v", err, smsparser.ErrCantParse)
		}
	})
}
