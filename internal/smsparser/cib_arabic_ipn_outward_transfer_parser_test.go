package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicIPNOutwardTransferParser(t *testing.T) {
	tests := []struct {
		name              string
		sms               string
		expectedAmount    float64
		expectedDate      string
		expectedAccountID string
		expectedCurrency  string
		expectedRef       string
	}{
		{
			name:              "Transfer 350 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    350.00,
			expectedDate:      "08-11-2025 21:51",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "s5t6u7v8",
		},
		{
			name:              "Transfer 275 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 275.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي w9x0y1z2 بتاريخ 06-11-2025 21:37 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    275.00,
			expectedDate:      "06-11-2025 21:37",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "w9x0y1z2",
		},
		{
			name:              "Transfer 125 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 125.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي a3b4c5d6 بتاريخ 07-11-2025 20:46 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    125.00,
			expectedDate:      "07-11-2025 20:46",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "a3b4c5d6",
		},
		{
			name:              "Transfer 900 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 900.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي e7f8g9h0 بتاريخ 15-07-2025 09:31 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    900.00,
			expectedDate:      "15-07-2025 09:31",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "e7f8g9h0",
		},
		{
			name:              "Transfer 1500 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1500.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي q9r0s1t2 بتاريخ 11-07-2025 20:07 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    1500.00,
			expectedDate:      "11-07-2025 20:07",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "q9r0s1t2",
		},
		{
			name:              "Transfer 750 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 750.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي u3v4w5x6 بتاريخ 10-07-2025 02:29 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    750.00,
			expectedDate:      "10-07-2025 02:29",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "u3v4w5x6",
		},
		{
			name:              "Transfer 375.50 EGP",
			sms:               "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 375.50 جم من حسابك المنتهي بـ ********1234 برقم مرجعي g5h6i7j8 بتاريخ 10-11-2025 20:57 للمزيد، برجاء الاتصال بـ 19666",
			expectedAmount:    375.50,
			expectedDate:      "10-11-2025 20:57",
			expectedAccountID: "1234",
			expectedCurrency:  "EGP",
			expectedRef:       "g5h6i7j8",
		},
	}

	parser := &smsparser.CIBArabicIPNOutwardTransferParser{}

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

			if result.TxKind != models.ExpenseTxKind {
				t.Errorf("TxKind = %v, want %v", result.TxKind, models.ExpenseTxKind)
			}

			if result.Amount != tt.expectedAmount {
				t.Errorf("Amount = %v, want %v", result.Amount, tt.expectedAmount)
			}

			if result.Currency != tt.expectedCurrency {
				t.Errorf("Currency = %q, want %q", result.Currency, tt.expectedCurrency)
			}

			if result.AccountID != tt.expectedAccountID {
				t.Errorf("AccountID = %q, want %q", result.AccountID, tt.expectedAccountID)
			}

			if result.Ref != tt.expectedRef {
				t.Errorf("Ref = %q, want %q", result.Ref, tt.expectedRef)
			}

			expectedTime, _ := time.Parse("02-01-2006 15:04", tt.expectedDate)
			if !result.Timestamp.Equal(expectedTime) {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, expectedTime)
			}

			if result.Counterparty != "" {
				t.Errorf("Counterparty = %q, want empty string (outward transfers don't specify recipient)", result.Counterparty)
			}

			if result.ToAmount != 0 {
				t.Errorf("ToAmount = %v, want 0", result.ToAmount)
			}

			if result.ToAccountID != "" {
				t.Errorf("ToAccountID = %q, want empty string", result.ToAccountID)
			}

			if result.ToCurrency != "" {
				t.Errorf("ToCurrency = %q, want empty string", result.ToCurrency)
			}
		})
	}
}

func TestCIBArabicIPNOutwardTransferParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicIPNOutwardTransferParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB IPN outward transfer SMS",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666",
			want: true,
		},
		{
			name: "Inward transfer (not outward)",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
			want: false,
		},
		{
			name: "Debit card purchase (not transfer)",
			sms:  "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			want: false,
		},
		{
			name: "Empty string",
			sms:  "",
			want: false,
		},
		{
			name: "Different bank",
			sms:  "From HSBC: Transfer sent",
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

func TestCIBArabicIPNOutwardTransferParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicIPNOutwardTransferParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "يرجى العلم انه تم تنفيذ تحويل لحظي",
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
