package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicATMWithdrawalParser(t *testing.T) {
	tests := []struct {
		name                         string
		sms                          string
		expectedCounterparty         string
		expectedAmount               float64
		expectedCurrency             string
		expectedToAmount             float64
		expectedToCurrency           string
		expectedAvailableBalance     float64
		expectedDate                 string
		expectedAccountID            string
		expectedAvailableBalanceCurr string
	}{
		{
			name:                         "CIB BRANCH A ATM withdrawal 500",
			sms:                          "تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00",
			expectedCounterparty:         "CIB BRANCH A ATM",
			expectedAmount:               500.00,
			expectedToAmount:             500.00,
			expectedAvailableBalance:     4500.00,
			expectedDate:                 "15/10/25 18:12",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "NBE ATM withdrawal 1000",
			sms:                          "تم سحب مبلغ  EGP 1000.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من        NBE ATM123 في 09/10/25 21:53 ، الرصيد المتاح EGP 3500.00",
			expectedCounterparty:         "NBE ATM123",
			expectedAmount:               1000.00,
			expectedToAmount:             1000.00,
			expectedAvailableBalance:     3500.00,
			expectedDate:                 "09/10/25 21:53",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "ADCB withdrawal 750",
			sms:                          "تم سحب مبلغ  EGP 750.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من           ADCB في 09/10/25 02:08 ، الرصيد المتاح EGP 2750.00",
			expectedCounterparty:         "ADCB",
			expectedAmount:               750.00,
			expectedToAmount:             750.00,
			expectedAvailableBalance:     2750.00,
			expectedDate:                 "09/10/25 02:08",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "CIB BRANCH B ATM withdrawal 1500",
			sms:                          "تم سحب مبلغ  EGP 1500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH B ATM في 06/10/25 14:34 ، الرصيد المتاح EGP 1250.00",
			expectedCounterparty:         "CIB BRANCH B ATM",
			expectedAmount:               1500.00,
			expectedToAmount:             1500.00,
			expectedAvailableBalance:     1250.00,
			expectedDate:                 "06/10/25 14:34",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "CIB BRANCH F ATM withdrawal 1200",
			sms:                          "تم سحب مبلغ  EGP 1200.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH F ATM في 18/08/25 18:20 ، الرصيد المتاح EGP 5900.00",
			expectedCounterparty:         "CIB BRANCH F ATM",
			expectedAmount:               1200.00,
			expectedToAmount:             1200.00,
			expectedAvailableBalance:     5900.00,
			expectedDate:                 "18/08/25 18:20",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "CIB BRANCH H ATM withdrawal 400",
			sms:                          "تم سحب مبلغ  EGP 400.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH H ATM في 04/12/25 13:31 ، الرصيد المتاح EGP 4600.00",
			expectedCounterparty:         "CIB BRANCH H ATM",
			expectedAmount:               400.00,
			expectedToAmount:             400.00,
			expectedAvailableBalance:     4600.00,
			expectedDate:                 "04/12/25 13:31",
			expectedAccountID:            "**1234",
			expectedCurrency:             "EGP",
			expectedToCurrency:           "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
	}

	parser := &smsparser.CIBArabicATMWithdrawalParser{}

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

			if result.Counterparty != tt.expectedCounterparty {
				t.Errorf("Counterparty = %q, want %q", result.Counterparty, tt.expectedCounterparty)
			}

			if result.Amount != tt.expectedAmount {
				t.Errorf("Amount = %v, want %v", result.Amount, tt.expectedAmount)
			}

			if result.Currency != tt.expectedCurrency {
				t.Errorf("Currency = %q, want %q", result.Currency, tt.expectedCurrency)
			}

			if result.AvailableBalance != tt.expectedAvailableBalance {
				t.Errorf("AvailableBalance = %v, want %v", result.AvailableBalance, tt.expectedAvailableBalance)
			}

			if result.AvailableBalanceCurrency != tt.expectedAvailableBalanceCurr {
				t.Errorf("AvailableBalanceCurrency = %q, want %q", result.AvailableBalanceCurrency, tt.expectedAvailableBalanceCurr)
			}

			if result.AccountID != tt.expectedAccountID {
				t.Errorf("AccountID = %q, want %q", result.AccountID, tt.expectedAccountID)
			}

			expectedTime, _ := time.Parse("02/01/06 15:04", tt.expectedDate)
			if !result.Timestamp.Equal(expectedTime) {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, expectedTime)
			}

			if result.ToAmount != tt.expectedToAmount {
				t.Errorf("ToAmount = %v, want %v", result.ToAmount, tt.expectedToAmount)
			}

			if result.ToAccountID != "Cash" {
				t.Errorf("ToAccountID = %q, want Cash", result.ToAccountID)
			}

			if result.ToCurrency != tt.expectedToCurrency {
				t.Errorf("ToCurrency = %q, want %q", result.ToCurrency, tt.expectedToCurrency)
			}
		})
	}
}

func TestCIBArabicATMWithdrawalParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicATMWithdrawalParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB ATM withdrawal SMS",
			sms:  "تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00",
			want: true,
		},
		{
			name: "Debit card purchase (not withdrawal)",
			sms:  "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			want: false,
		},
		{
			name: "IPN transfer (not withdrawal)",
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
			sms:  "From HSBC: ATM withdrawal",
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

func TestCIBArabicATMWithdrawalParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicATMWithdrawalParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "تم سحب مبلغ",
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
