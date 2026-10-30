package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicRefundParser(t *testing.T) {
	tests := []struct {
		name                 string
		sms                  string
		expectedCounterparty string
		expectedAmount       float64
		expectedDate         string
		expectedAccountID    string
		expectedCurrency     string
	}{
		{
			name:                 "Online Marketplace refund 125 EGP",
			sms:                  "تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ REFUND على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025",
			expectedCounterparty: "ONLINE MARKETPLACE STORE XYZ REFUND",
			expectedAmount:       125.00,
			expectedDate:         "27-07-2025",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
		},
		{
			name:                 "Electronics Retailer refund 2450 EGP",
			sms:                  "تم اضافة المعاملة الخاصة بـ ELECTRONICS RETAILER ABC ONLINE SHOP على بطاقتكم المنتهية بـ 1234 بقيمة EGP 2,450.00 بتاريخ 13-07-2025",
			expectedCounterparty: "ELECTRONICS RETAILER ABC ONLINE SHOP",
			expectedAmount:       2450.00,
			expectedDate:         "13-07-2025",
			expectedAccountID:    "1234",
			expectedCurrency:     "EGP",
		},
	}

	parser := &smsparser.CIBArabicRefundParser{}

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

			if result.TxKind != models.RefundTxKind {
				t.Errorf("TxKind = %v, want %v", result.TxKind, models.RefundTxKind)
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

			expectedTime, _ := time.Parse("02-01-2006", tt.expectedDate)
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

func TestCIBArabicRefundParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicRefundParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB refund SMS",
			sms:  "تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ REFUND على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025",
			want: true,
		},
		{
			name: "Hold release (not refund)",
			sms:  "تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10",
			want: false,
		},
		{
			name: "Debit card purchase (not refund)",
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
			sms:  "From HSBC: Refund received",
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

func TestCIBArabicRefundParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicRefundParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ REFUND على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "تم اضافة المعاملة الخاصة بـ",
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
