package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestHSBCEnglishDebitCardPurchaseParser(t *testing.T) {
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
			name:                         "MERCHANT1 Locally purchase",
			sms:                          "From HSBC: 28JUL25 MERCHANT1*LOCALLY Purchase from 123-456***-789 USD 125.50- Your available balance is USD 5,432.10",
			expectedCounterparty:         "MERCHANT1*LOCALLY",
			expectedAmount:               125.50,
			expectedAvailableBalance:     5432.10,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Payment processor purchase",
			sms:                          "From HSBC: 28JUL25 PAYPRO*CS Purchase from 123-456***-789 USD 89.99- Your available balance is USD 5,342.11",
			expectedCounterparty:         "PAYPRO*CS",
			expectedAmount:               89.99,
			expectedAvailableBalance:     5342.11,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Store purchase",
			sms:                          "From HSBC: 28JUL25 RetailStore Purchase from 123-456***-789 USD 450.00- Your available balance is USD 4,892.11",
			expectedCounterparty:         "RetailStore",
			expectedAmount:               450.00,
			expectedAvailableBalance:     4892.11,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Payment processor gym purchase",
			sms:                          "From HSBC: 02AUG25 PAYPRO-*FITNESS CTR Purchase from 123-456***-789 USD 75.00- Your available balance is USD 4,817.11",
			expectedCounterparty:         "PAYPRO-*FITNESS CTR",
			expectedAmount:               75.00,
			expectedAvailableBalance:     4817.11,
			expectedDate:                 "02Aug25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Restaurant purchase",
			sms:                          "From HSBC: 25AUG25 MERCHANT2*RESTAURANT Purchase from 123-456***-789 USD 42.75- Your available balance is USD 4,774.36",
			expectedCounterparty:         "MERCHANT2*RESTAURANT",
			expectedAmount:               42.75,
			expectedAvailableBalance:     4774.36,
			expectedDate:                 "25Aug25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Payment processor shop with spaces in merchant name",
			sms:                          "From HSBC: 26AUG25 PAYPRO-*SHOP FOR GOODS Purchase from 123-456***-789 USD 220.00- Your available balance is USD 4,554.36",
			expectedCounterparty:         "PAYPRO-*SHOP FOR GOODS",
			expectedAmount:               220.00,
			expectedAvailableBalance:     4554.36,
			expectedDate:                 "26Aug25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Retail chain purchase",
			sms:                          "From HSBC: 26AUG25 RETAIL*SHOE STORE Purchase from 123-456***-789 USD 135.00- Your available balance is USD 4,419.36",
			expectedCounterparty:         "RETAIL*SHOE STORE",
			expectedAmount:               135.00,
			expectedAvailableBalance:     4419.36,
			expectedDate:                 "26Aug25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Supermarket purchase with long merchant name",
			sms:                          "From HSBC: 23SEP25 SUPERMARKET DOWNTOWN Purchase from 123-456***-789 USD 68.25- Your available balance is USD 4,351.11",
			expectedCounterparty:         "SUPERMARKET DOWNTOWN",
			expectedAmount:               68.25,
			expectedAvailableBalance:     4351.11,
			expectedDate:                 "23Sep25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Bakery purchase",
			sms:                          "From HSBC: 08OCT25 THE BAKERY Purchase from 123-456***-789 USD 385.00- Your available balance is USD 3,966.11",
			expectedCounterparty:         "THE BAKERY",
			expectedAmount:               385.00,
			expectedAvailableBalance:     3966.11,
			expectedDate:                 "08Oct25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Payment processor gym November purchase",
			sms:                          "From HSBC: 01NOV25 PAYPRO-*FITNESS CTR Purchase from 123-456***-789 USD 150.00- Your available balance is USD 3,816.11",
			expectedCounterparty:         "PAYPRO-*FITNESS CTR",
			expectedAmount:               150.00,
			expectedAvailableBalance:     3816.11,
			expectedDate:                 "01Nov25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
		{
			name:                         "Numeric merchant ID",
			sms:                          "From HSBC: 02NOV25 5847 Purchase from 123-456***-789 USD 95.80- Your available balance is USD 3,720.31",
			expectedCounterparty:         "5847",
			expectedAmount:               95.80,
			expectedAvailableBalance:     3720.31,
			expectedDate:                 "02Nov25",
			expectedAccountID:            "123-456***-789",
			expectedCurrency:             "USD",
			expectedAvailableBalanceCurr: "USD",
		},
	}

	parser := &smsparser.HSBCEnglishDebitCardPurchaseParser{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !parser.CanParse(tt.sms) {
				t.Errorf("CanParse() = false, want true")
			}

			result, err := parser.Parse(tt.sms)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if result.Bank != smsparser.HSBC {
				t.Errorf("Bank = %v, want %v", result.Bank, smsparser.HSBC)
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

			expectedTime, _ := time.Parse("02Jan06", tt.expectedDate)
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

func TestHSBCEnglishDebitCardPurchaseParser_CanParse(t *testing.T) {
	parser := &smsparser.HSBCEnglishDebitCardPurchaseParser{}

	tests := []struct {
		name string
		sms  string
		want bool
	}{
		{
			name: "Valid HSBC purchase SMS",
			sms:  "From HSBC: 02NOV25 5847 Purchase from 123-456***-789 USD 95.80- Your available balance is USD 3,720.31",
			want: true,
		},
		{
			name: "HSBC deposit (not purchase)",
			sms:  "From HSBC: 02NOV25 Deposit to 123-456***-789 USD 500.00+ Your available balance is USD 4,220.31",
			want: false,
		},
		{
			name: "Different bank",
			sms:  "From OtherBank: 02NOV25 Purchase USD 100.00",
			want: false,
		},
		{
			name: "Missing 'Purchase from'",
			sms:  "From HSBC: 02NOV25 5847 USD 95.80- Your available balance is USD 3,720.31",
			want: false,
		},
		{
			name: "Empty string",
			sms:  "",
			want: false,
		},
		{
			name: "HSBC but no prefix",
			sms:  "HSBC: 02NOV25 5847 Purchase from 123-456***-789 USD 95.80-",
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

func TestHSBCEnglishDebitCardPurchaseParser_Parse_Errors(t *testing.T) {
	parser := &smsparser.HSBCEnglishDebitCardPurchaseParser{}

	tests := []struct {
		name    string
		sms     string
		wantErr bool
	}{
		{
			name:    "Valid SMS should not error",
			sms:     "From HSBC: 02NOV25 5847 Purchase from 123-456***-789 USD 95.80- Your available balance is USD 3,720.31",
			wantErr: false,
		},
		{
			name:    "Invalid date format returns error",
			sms:     "From HSBC: 99ZZZ99 5847 Purchase from 123-456***-789 USD 95.80- Your available balance is USD 3,720.31",
			wantErr: false,
		},
		{
			name:    "Too few fields",
			sms:     "From HSBC: 02NOV25",
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
