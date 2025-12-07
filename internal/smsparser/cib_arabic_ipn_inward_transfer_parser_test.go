package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicIPNInwardTransferParser(t *testing.T) {
	tests := []struct {
		name                 string
		sms                  string
		expectedCounterparty string
		expectedAmount       float64
		expectedDate         string
		expectedAccountID    string
		expectedCurrency     string
		expectedRef          string
	}{
		{
			name:                 "Transfer from JOHN DOE",
			sms:                  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
			expectedCounterparty: "JOHN DOE SMITH",
			expectedAmount:       1000.00,
			expectedDate:         "07-11-2025 16:54",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
			expectedRef:          "a1b2c3d4",
		},
		{
			name:                 "Transfer from JANE JOHNSON",
			sms:                  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 500.00 جم إلى حسابك المنتهي بـ ********1234 من JANE MARY JOHNSON برقم مرجعي e5f6g7h8 بتاريخ 07-11-2025 16:55 للمزيد، برجاء الاتصال بـ 19666",
			expectedCounterparty: "JANE MARY JOHNSON",
			expectedAmount:       500.00,
			expectedDate:         "07-11-2025 16:55",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
			expectedRef:          "e5f6g7h8",
		},
		{
			name:                 "Transfer from MICHAEL BROWN",
			sms:                  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 750.00 جم إلى حسابك المنتهي بـ ********1234 من MICHAEL ROBERT BROWN برقم مرجعي i9j0k1l2 بتاريخ 07-11-2025 21:57 للمزيد، برجاء الاتصال بـ 19666",
			expectedCounterparty: "MICHAEL ROBERT BROWN",
			expectedAmount:       750.00,
			expectedDate:         "07-11-2025 21:57",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
			expectedRef:          "i9j0k1l2",
		},
		{
			name:                 "Small transfer amount",
			sms:                  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 250.00 جم إلى حسابك المنتهي بـ ********1234 من SARAH ELIZABETH DAVIS برقم مرجعي m3n4o5p6 بتاريخ 08-11-2025 01:02 للمزيد، برجاء الاتصال بـ 19666",
			expectedCounterparty: "SARAH ELIZABETH DAVIS",
			expectedAmount:       250.00,
			expectedDate:         "08-11-2025 01:02",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
			expectedRef:          "m3n4o5p6",
		},
		{
			name:                 "Transfer from DAVID WILSON",
			sms:                  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1200.00 جم إلى حسابك المنتهي بـ ********1234 من DAVID WILLIAM WILSON برقم مرجعي q7r8s9t0 بتاريخ 08-11-2025 01:04 للمزيد، برجاء الاتصال بـ 19666",
			expectedCounterparty: "DAVID WILLIAM WILSON",
			expectedAmount:       1200.00,
			expectedDate:         "08-11-2025 01:04",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
			expectedRef:          "q7r8s9t0",
		},
	}

	parser := &smsparser.CIBArabicIPNInwardTransferParser{}

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

			if result.TxKind != models.IncomeTxKind {
				t.Errorf("TxKind = %v, want %v", result.TxKind, models.IncomeTxKind)
			}

			if result.Counterparty != tt.expectedCounterparty {
				t.Errorf("Counterparty = %q, want %q", result.Counterparty, tt.expectedCounterparty)
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

func TestCIBArabicIPNInwardTransferParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicIPNInwardTransferParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB IPN inward transfer SMS",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
			want: true,
		},
		{
			name: "Outward transfer (not inward)",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666",
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
			sms:  "From HSBC: Transfer received",
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

func TestCIBArabicIPNInwardTransferParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicIPNInwardTransferParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
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
