package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicDebitToCreditTransferParser(t *testing.T) {
	tests := []struct {
		name              string
		sms               string
		expectedAmount    float64
		expectedDate      string
		expectedToAccount string
		expectedCurrency  string
	}{
		{
			name:              "Arabic format payment to credit card 500",
			sms:               "تم سداد مبلغ 500.00 جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ 05-12-25",
			expectedAmount:    500.00,
			expectedDate:      "05-12-25",
			expectedToAccount: "5678",
			expectedCurrency:  "EGP",
		},
		{
			name:              "Arabic format payment to credit card 1200",
			sms:               "تم سداد مبلغ 1200.00 جم فى بطاقتكم الائتمانية المنتهية بـ 9876 بتاريخ 15-11-25",
			expectedAmount:    1200.00,
			expectedDate:      "15-11-25",
			expectedToAccount: "9876",
			expectedCurrency:  "EGP",
		},
	}

	parser := &smsparser.CIBArabicDebitToCreditTransferParser{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !parser.CanParse(tt.sms) {
				t.Errorf("CanParse() = false, want true")
			}

			result, err := parser.Parse(tt.sms)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if result.Bank != smsparser.CIB {
				t.Errorf("Bank = %v, want %v", result.Bank, smsparser.CIB)
			}

			if result.TxKind != models.TransferTxKind {
				t.Errorf("TxKind = %v, want %v", result.TxKind, models.TransferTxKind)
			}

			if result.Amount != tt.expectedAmount {
				t.Errorf("Amount = %v, want %v", result.Amount, tt.expectedAmount)
			}

			if result.Currency != tt.expectedCurrency {
				t.Errorf("Currency = %q, want %q", result.Currency, tt.expectedCurrency)
			}

			if result.ToAmount != tt.expectedAmount {
				t.Errorf("ToAmount = %v, want %v", result.ToAmount, tt.expectedAmount)
			}

			if result.ToCurrency != tt.expectedCurrency {
				t.Errorf("ToCurrency = %q, want %q", result.ToCurrency, tt.expectedCurrency)
			}

			if result.ToAccountID != tt.expectedToAccount {
				t.Errorf("ToAccountID = %q, want %q", result.ToAccountID, tt.expectedToAccount)
			}

			expectedTime, _ := time.Parse("02-01-06", tt.expectedDate)
			if !result.Timestamp.IsZero() && !result.Timestamp.Equal(expectedTime) {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, expectedTime)
			}
		})
	}
}

func TestCIBArabicDebitToCreditTransferParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicDebitToCreditTransferParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB Arabic debit to credit transfer SMS",
			sms:  "تم سداد مبلغ 500.00 جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ 05-12-25",
			want: true,
		},
		{
			name: "English format debit transfer (not Arabic)",
			sms:  "Your account ending with ********1234 is debited with amount EGP 750.00DR On 07 DEC 2025 with transfer to  another account.  For more information, please contact 19666.",
			want: false,
		},
		{
			name: "Debit card purchase (not transfer)",
			sms:  "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			want: false,
		},
		{
			name: "IPN transfer (not debit to credit)",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
			want: false,
		},
		{
			name: "Empty string",
			sms:  "",
			want: false,
		},
		{
			name: "Different bank",
			sms:  "From HSBC: Transfer completed",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parser.CanParse(tt.sms); got != tt.want {
				t.Errorf("CanParse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIBArabicDebitToCreditTransferParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicDebitToCreditTransferParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "تم سداد مبلغ 500.00 جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ 05-12-25",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "تم سداد مبلغ",
			wantErr: true,
		},
		{
			name:    "Empty SMS",
			sms:     "",
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
