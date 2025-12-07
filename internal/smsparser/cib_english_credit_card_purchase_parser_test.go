package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBEnglishCreditCardPurchaseParser(t *testing.T) {
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
			name:                         "Generic Store ABC purchase",
			sms:                          "Your credit card ending with#5678 was charged for EGP 150.00 at Generic Store ABC on 04/12/25  at 10:48. Card available limit is EGP  95000.00. For more details, please visit https://cib.eg/mb",
			expectedCounterparty:         "Generic Store ABC",
			expectedAmount:               150.00,
			expectedAvailableBalance:     95000.00,
			expectedDate:                 "04/12/25 10:48",
			expectedAccountID:            "5678",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
	}

	parser := &smsparser.CIBEnglishCreditCardPurchaseParser{}

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

func TestCIBEnglishCreditCardPurchaseParser_CanParse(t *testing.T) {
	parser := &smsparser.CIBEnglishCreditCardPurchaseParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid CIB credit card purchase SMS",
			sms:  "Your credit card ending with#5678 was charged for EGP 150.00 at Generic Store ABC on 04/12/25  at 10:48. Card available limit is EGP  95000.00. For more details, please visit https://cib.eg/mb",
			want: true,
		},
		{
			name: "Debit card purchase (not credit)",
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
			sms:  "From HSBC: Purchase made",
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

func TestCIBEnglishCreditCardPurchaseParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.CIBEnglishCreditCardPurchaseParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "Your credit card ending with#5678 was charged for EGP 150.00 at Generic Store ABC on 04/12/25  at 10:48. Card available limit is EGP  95000.00. For more details, please visit https://cib.eg/mb",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			sms:     "Your credit card ending with#5678",
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
