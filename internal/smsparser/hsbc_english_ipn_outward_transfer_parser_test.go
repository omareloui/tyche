package smsparser

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestHSBCEnglishIPNOutwardTransferParser_CanParse(t *testing.T) {
	parser := &HSBCEnglishIPNOutwardTransferParser{}

	tests := []struct {
		name     string
		sms      string
		expected bool
	}{
		{
			name:     "valid HSBC IPN outward transfer SMS",
			sms:      "Your HSBC Account ********1234 was debited with IPN outward transfer for EGP 67,020.00 on 27-07-2025 12:54 to JOHN DOE SMITH with reference 199b2b61. For further details, please contact HSBC call centre",
			expected: true,
		},
		{
			name:     "valid HSBC IPN outward transfer with numeric counterparty",
			sms:      "Your HSBC Account ********1234 was debited with IPN outward transfer for EGP 6,731.73 on 29-07-2025 21:24 to 7967 with reference 4f38876c. For further details, please contact HSBC call centre",
			expected: true,
		},
		{
			name:     "invalid SMS - different prefix",
			sms:      "Your Credit Card ending with *** 5450 has been used for AED 30.00",
			expected: false,
		},
		{
			name:     "invalid SMS - missing IPN outward transfer",
			sms:      "Your HSBC Account ********1234 was debited for EGP 100.00",
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

func TestHSBCEnglishIPNOutwardTransferParser_Parse(t *testing.T) {
	parser := &HSBCEnglishIPNOutwardTransferParser{}

	tests := []struct {
		name    string
		sms     string
		want    *SMS
		wantErr bool
	}{
		{
			name: "valid SMS with full name counterparty",
			sms:  "Your HSBC Account ********1234 was debited with IPN outward transfer for EGP 67,020.00 on 27-07-2025 12:54 to JOHN DOE SMITH with reference 199b2b61. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "1234",
				Currency:     "EGP",
				Amount:       67020.00,
				Timestamp:    time.Date(2025, 7, 27, 12, 54, 0, 0, time.UTC),
				Counterparty: "JOHN DOE SMITH",
				Ref:          "199b2b61",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with numeric counterparty",
			sms:  "Your HSBC Account ********5678 was debited with IPN outward transfer for EGP 6,731.73 on 29-07-2025 21:24 to 7967 with reference 4f38876c. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "5678",
				Currency:     "EGP",
				Amount:       6731.73,
				Timestamp:    time.Date(2025, 7, 29, 21, 24, 0, 0, time.UTC),
				Counterparty: "7967",
				Ref:          "4f38876c",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with lowercase counterparty",
			sms:  "Your HSBC Account ********9012 was debited with IPN outward transfer for EGP 5,005.00 on 02-08-2025 18:55 to jane smith williams with reference ff9d07c7. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "9012",
				Currency:     "EGP",
				Amount:       5005.00,
				Timestamp:    time.Date(2025, 8, 2, 18, 55, 0, 0, time.UTC),
				Counterparty: "jane smith williams",
				Ref:          "ff9d07c7",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with small amount",
			sms:  "Your HSBC Account ********3456 was debited with IPN outward transfer for EGP 200.50 on 12-08-2025 18:32 to 7773 with reference 8eb5c74f. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "3456",
				Currency:     "EGP",
				Amount:       200.50,
				Timestamp:    time.Date(2025, 8, 12, 18, 32, 0, 0, time.UTC),
				Counterparty: "7773",
				Ref:          "8eb5c74f",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with large amount",
			sms:  "Your HSBC Account ********7890 was debited with IPN outward transfer for EGP 17,247.23 on 24-09-2025 01:37 to ALICE JOHNSON BROWN with reference b8b97336. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "7890",
				Currency:     "EGP",
				Amount:       17247.23,
				Timestamp:    time.Date(2025, 9, 24, 1, 37, 0, 0, time.UTC),
				Counterparty: "ALICE JOHNSON BROWN",
				Ref:          "b8b97336",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with different account",
			sms:  "Your HSBC Account ********2468 was debited with IPN outward transfer for EGP 1,001.00 on 14-09-2025 17:31 to 8821 with reference f204be32. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "2468",
				Currency:     "EGP",
				Amount:       1001.00,
				Timestamp:    time.Date(2025, 9, 14, 17, 31, 0, 0, time.UTC),
				Counterparty: "8821",
				Ref:          "f204be32",
			},
			wantErr: false,
		},
		{
			name: "valid SMS with mixed case name",
			sms:  "Your HSBC Account ********1357 was debited with IPN outward transfer for EGP 3,303.30 on 02-11-2025 14:48 to ROBERT TAYLOR DAVIS with reference b97d731c. For further details, please contact HSBC call centre",
			want: &SMS{
				Bank:         HSBC,
				TxKind:       models.ExpenseTxKind,
				AccountID:    "1357",
				Currency:     "EGP",
				Amount:       3303.30,
				Timestamp:    time.Date(2025, 11, 2, 14, 48, 0, 0, time.UTC),
				Counterparty: "ROBERT TAYLOR DAVIS",
				Ref:          "b97d731c",
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
			name:    "invalid SMS - wrong format",
			sms:     "Your Credit Card ending with *** 5450 has been used for AED 30.00",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid SMS - malformed",
			sms:     "Your HSBC Account ********1234 was debited",
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
		})
	}
}
