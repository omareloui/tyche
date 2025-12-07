package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBArabicDebitCardPurchaseParser(t *testing.T) {
	tests := []struct {
		name                         string
		sms                          string
		expectedCounterparty         string
		expectedAmount               float64
		expectedAvailableBalance     float64
		expectedDate                 string
		expectedAccountID            string
		expectedCurrency             string
		expectedAvailableBalanceCurr string
	}{
		{
			name:                         "Generic Store purchase",
			sms:                          "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			expectedCounterparty:         "Generic Store",
			expectedAmount:               125.50,
			expectedAvailableBalance:     5432.10,
			expectedDate:                 "05/07/25 21:45",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Online Shop purchase",
			sms:                          "تم خصم مبلغ  EGP 89.99 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Online Shop في 05/07/25 22:09 ، الرصيد المتاح 5342.11 EGP",
			expectedCounterparty:         "Online Shop",
			expectedAmount:               89.99,
			expectedAvailableBalance:     5342.11,
			expectedDate:                 "05/07/25 22:09",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Hotel Chain purchase",
			sms:                          "تم خصم مبلغ  EGP 450.00 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Hotel Chain في 07/07/25 13:40 ، الرصيد المتاح 4892.11 EGP",
			expectedCounterparty:         "Hotel Chain",
			expectedAmount:               450.00,
			expectedAvailableBalance:     4892.11,
			expectedDate:                 "07/07/25 13:40",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Electronics Store purchase",
			sms:                          "تم خصم مبلغ  EGP 220.00 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Electronics Store في 09/07/25 13:52 ، الرصيد المتاح 4597.11 EGP",
			expectedCounterparty:         "Electronics Store",
			expectedAmount:               220.00,
			expectedAvailableBalance:     4597.11,
			expectedDate:                 "09/07/25 13:52",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Restaurant purchase",
			sms:                          "تم خصم مبلغ  EGP 42.75 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Restaurant في 10/07/25 11:37 ، الرصيد المتاح 4554.36 EGP",
			expectedCounterparty:         "Restaurant",
			expectedAmount:               42.75,
			expectedAvailableBalance:     4554.36,
			expectedDate:                 "10/07/25 11:37",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Grocery Store purchase",
			sms:                          "تم خصم مبلغ  EGP 68.25 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Grocery Store في 13/07/25 16:17 ، الرصيد المتاح 4351.11 EGP",
			expectedCounterparty:         "Grocery Store",
			expectedAmount:               68.25,
			expectedAvailableBalance:     4351.11,
			expectedDate:                 "13/07/25 16:17",
			expectedAccountID:            "1234",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
	}

	parser := &smsparser.CIBArabicDebitCardPurchaseParser{}

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

func TestCIBArabicDebitCardPurchaseParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBArabicDebitCardPurchaseParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB debit purchase SMS",
			sms:  "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			want: true,
		},
		{
			name: "ATM withdrawal (not purchase)",
			sms:  "تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00",
			want: false,
		},
		{
			name: "IPN transfer (not purchase)",
			sms:  "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666",
			want: false,
		},
		{
			name: "NBE ATM withdrawal (excluded)",
			sms:  "تم سحب مبلغ  EGP 1000.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من        NBE ATM123 في 09/10/25 21:53 ، الرصيد المتاح EGP 3500.00",
			want: false,
		},
		{
			name: "Empty string",
			sms:  "",
			want: false,
		},
		{
			name: "Different bank",
			sms:  "From HSBC: 02NOV25 Purchase USD 100.00",
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

func TestCIBArabicDebitCardPurchaseParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBArabicDebitCardPurchaseParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "تم خصم مبلغ",
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
