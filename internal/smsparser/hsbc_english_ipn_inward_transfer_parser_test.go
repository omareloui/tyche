package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishIPNInwardTransferParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishIPNInwardTransferParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC IPN inward transfer SMS",
			sms:      "Your HSBC Account ********1234 was credited with IPN inward transfer for USD 500.00 on 31-08-2025 18:31 from JOHN DOE with reference abc123xyz. For further details, please contact HSBC call centre",
			expected: true,
		},
		{
			name:     "valid HSBC IPN inward transfer with multi-word sender",
			sms:      "Your HSBC Account ********5678 was credited with IPN inward transfer for EUR 1200.50 on 15-12-2024 14:22 from JANE MARIE SMITH with reference ref456def. For further details, please contact HSBC call centre",
			expected: true,
		},
		{
			name:     "invalid SMS - different prefix",
			sms:      "Your HSBC Card ********1234 was credited with IPN inward transfer for USD 500.00",
			expected: false,
		},
		{
			name:     "invalid SMS - missing 'was credited with IPN inward transfer for'",
			sms:      "Your HSBC Account ********1234 received USD 500.00",
			expected: false,
		},
		{
			name:     "invalid SMS - outward transfer instead",
			sms:      "Your HSBC Account ********1234 was debited with IPN outward transfer for USD 500.00",
			expected: false,
		},
		{
			name:     "invalid SMS - completely different format",
			sms:      "Transfer successful. Amount: 500 USD",
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

func TestHSBCEnglishIPNInwardTransferParser_Parse(t *testing.T) {
	parser := &smsparser.HSBCEnglishIPNInwardTransferParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "valid SMS with two-word sender name",
			sms:  "Your HSBC Account ********1234 was credited with IPN inward transfer for USD 500.00 on 31-08-2025 18:31 from JOHN DOE with reference abc123xyz. For further details, please contact HSBC call centre",
			want: &smsparser.SMS{
				Bank:         smsparser.HSBC,
				TxKind:       models.IncomeTxKind,
				AccountID:    "1234",
				Currency:     "USD",
				Amount:       500.00,
				Timestamp:    time.Date(2025, 8, 31, 18, 31, 0, 0, time.UTC),
				Counterparty: "JOHN DOE",
				Ref:          "abc123xyz",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with four-word sender name",
			sms:  "Your HSBC Account ********5678 was credited with IPN inward transfer for EUR 1200.50 on 15-12-2024 14:22 from JANE MARIE LOUISE SMITH with reference ref456def. For further details, please contact HSBC call centre",
			want: &smsparser.SMS{
				Bank:         smsparser.HSBC,
				TxKind:       models.IncomeTxKind,
				AccountID:    "5678",
				Currency:     "EUR",
				Amount:       1200.50,
				Timestamp:    time.Date(2024, 12, 15, 14, 22, 0, 0, time.UTC),
				Counterparty: "JANE MARIE LOUISE SMITH",
				Ref:          "ref456def",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with single-word sender name",
			sms:  "Your HSBC Account ********9012 was credited with IPN inward transfer for GBP 750.25 on 01-01-2025 09:15 from COMPANY with reference xyz789abc. For further details, please contact HSBC call centre",
			want: &smsparser.SMS{
				Bank:         smsparser.HSBC,
				TxKind:       models.IncomeTxKind,
				AccountID:    "9012",
				Currency:     "GBP",
				Amount:       750.25,
				Timestamp:    time.Date(2025, 1, 1, 9, 15, 0, 0, time.UTC),
				Counterparty: "COMPANY",
				Ref:          "xyz789abc",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with large amount and comma",
			sms:  "Your HSBC Account ********3456 was credited with IPN inward transfer for USD 15,500.75 on 20-06-2025 12:00 from ROBERT WILLIAM ANDERSON with reference longref123. For further details, please contact HSBC call centre",
			want: &smsparser.SMS{
				Bank:         smsparser.HSBC,
				TxKind:       models.IncomeTxKind,
				AccountID:    "3456",
				Currency:     "USD",
				Amount:       15500.75,
				Timestamp:    time.Date(2025, 6, 20, 12, 0, 0, 0, time.UTC),
				Counterparty: "ROBERT WILLIAM ANDERSON",
				Ref:          "longref123",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with three-word sender name",
			sms:  "Your HSBC Account ********7890 was credited with IPN inward transfer for USD 2,300.00 on 10-03-2025 16:45 from MICHAEL JAMES BROWN with reference ref999. For further details, please contact HSBC call centre",
			want: &smsparser.SMS{
				Bank:         smsparser.HSBC,
				TxKind:       models.IncomeTxKind,
				AccountID:    "7890",
				Currency:     "USD",
				Amount:       2300.00,
				Timestamp:    time.Date(2025, 3, 10, 16, 45, 0, 0, time.UTC),
				Counterparty: "MICHAEL JAMES BROWN",
				Ref:          "ref999",
			},
			wantErr: false,
		},
		{
			name:    "invalid SMS - too short",
			sms:     "Your HSBC Account",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - malformed",
			sms:     "Your HSBC Account ********1234 was credited",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - wrong format",
			sms:     "Different bank transfer message",
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
			if got.Ref != tt.want.Ref {
				t.Errorf("Ref = %v, want %v", got.Ref, tt.want.Ref)
			}

			// Verify fields that should be empty for inward transfers
			if got.ToAmount != 0 {
				t.Errorf("ToAmount = %v, want 0", got.ToAmount)
			}
			if got.ToAccountID != "" {
				t.Errorf("ToAccountID = %q, want empty string", got.ToAccountID)
			}
			if got.ToCurrency != "" {
				t.Errorf("ToCurrency = %q, want empty string", got.ToCurrency)
			}
			if got.AvailableBalance != 0 {
				t.Errorf("AvailableBalance = %v, want 0", got.AvailableBalance)
			}
			if got.AvailableBalanceCurrency != "" {
				t.Errorf("AvailableBalanceCurrency = %q, want empty string", got.AvailableBalanceCurrency)
			}
		})
	}
}

func TestHSBCEnglishIPNInwardTransferParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.HSBCEnglishIPNInwardTransferParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "valid SMS should not error",
			sms:     "Your HSBC Account ********1234 was credited with IPN inward transfer for USD 500.00 on 31-08-2025 18:31 from JOHN DOE with reference abc123. For further details, please contact HSBC call centre",
			wantErr: false,
		},
		{
			name:    "too few fields",
			sms:     "Your HSBC Account ********1234",
			wantErr: true,
		},
		{
			name:    "empty SMS",
			sms:     "",
			wantErr: true,
		},
		{
			name:    "wrong format - not IPN inward",
			sms:     "Your HSBC Account ********1234 was debited for USD 500.00",
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
