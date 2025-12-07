package smsparser_test

import (
	"testing"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicHoldReleaseParser(t *testing.T) {
	tests := []struct {
		name                         string
		sms                          string
		expectedCounterparty         string
		expectedAmount               float64
		expectedAccountID            string
		expectedCurrency             string
		expectedAvailableBalance     float64
		expectedAvailableBalanceCurr string
	}{
		{
			name:                         "Ride Service A hold release",
			sms:                          "تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10",
			expectedCounterparty:         "Ride Service A",
			expectedAmount:               45.20,
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalance:     5432.10,
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "TAXI HOLD TEMP hold release",
			sms:                          "تم اضافة المعاملة من TAXI HOLD TEMP بقيمة 85.50 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5517.60",
			expectedCounterparty:         "TAXI HOLD TEMP",
			expectedAmount:               85.50,
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalance:     5517.60,
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Ride Service B hold release",
			sms:                          "تم اضافة المعاملة من Ride Service B بقيمة 32.00 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5549.60",
			expectedCounterparty:         "Ride Service B",
			expectedAmount:               32.00,
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalance:     5549.60,
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Delivery Service C hold release",
			sms:                          "تم اضافة المعاملة من Delivery Service C بقيمة 15.00 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5564.60",
			expectedCounterparty:         "Delivery Service C",
			expectedAmount:               15.00,
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalance:     5564.60,
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Mobility Service D hold release",
			sms:                          "تم اضافة المعاملة من Mobility Service D بقيمة 12.00 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5820.65",
			expectedCounterparty:         "Mobility Service D",
			expectedAmount:               12.00,
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalance:     5820.65,
			expectedAvailableBalanceCurr: "EGP",
		},
	}

	parser := &smsparser.CIBArabicHoldReleaseParser{}

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

			if result.AvailableBalance != tt.expectedAvailableBalance {
				t.Errorf("AvailableBalance = %v, want %v", result.AvailableBalance, tt.expectedAvailableBalance)
			}

			if result.AvailableBalanceCurrency != tt.expectedAvailableBalanceCurr {
				t.Errorf("AvailableBalanceCurrency = %q, want %q", result.AvailableBalanceCurrency, tt.expectedAvailableBalanceCurr)
			}

			// Hold releases don't have explicit refs
			if result.Ref != "" {
				t.Errorf("Ref = %q, want empty string", result.Ref)
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

func TestCIBArabicHoldReleaseParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicHoldReleaseParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB hold release SMS",
			sms:  "تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10",
			want: true,
		},
		{
			name: "Refund (not hold release)",
			sms:  "تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025",
			want: false,
		},
		{
			name: "Debit card purchase (not hold release)",
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
			sms:  "From HSBC: Hold released",
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

func TestCIBArabicHoldReleaseParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicHoldReleaseParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "تم اضافة المعاملة من",
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
