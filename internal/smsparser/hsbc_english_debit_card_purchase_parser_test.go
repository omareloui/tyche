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
			name:                         "GEIDEAE Locally purchase",
			sms:                          "From HSBC: 28JUL25 GEIDEAE*LOCALLY Purchase from 040-206***-001 EGP 888.06- Your available balance is EGP 28,017.29",
			expectedCounterparty:         "GEIDEAE*LOCALLY",
			expectedAmount:               888.06,
			expectedAvailableBalance:     28017.29,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Fawry purchase",
			sms:                          "From HSBC: 28JUL25 FAWRYPF*CS Purchase from 040-206***-001 EGP 899.00- Your available balance is EGP 27,118.29",
			expectedCounterparty:         "FAWRYPF*CS",
			expectedAmount:               899.00,
			expectedAvailableBalance:     27118.29,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Mazaya purchase",
			sms:                          "From HSBC: 28JUL25 Mazaya Purchase from 040-206***-001 EGP 5,480.00- Your available balance is EGP 21,638.29",
			expectedCounterparty:         "Mazaya",
			expectedAmount:               5480.00,
			expectedAvailableBalance:     21638.29,
			expectedDate:                 "28Jul25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Paymob Smash Gym purchase",
			sms:                          "From HSBC: 02AUG25 PAYMOB-*SMASH GYM Purchase from 040-206***-001 EGP 2,000.00- Your available balance is EGP 8,902.56",
			expectedCounterparty:         "PAYMOB-*SMASH GYM",
			expectedAmount:               2000.00,
			expectedAvailableBalance:     8902.56,
			expectedDate:                 "02Aug25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Geidea Papa Johns purchase",
			sms:                          "From HSBC: 25AUG25 GEIDEA*PAPA JOHNS Purchase from 040-206***-001 EGP 157.01- Your available balance is EGP 45,976.59",
			expectedCounterparty:         "GEIDEA*PAPA JOHNS",
			expectedAmount:               157.01,
			expectedAvailableBalance:     45976.59,
			expectedDate:                 "25Aug25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Paymob 2S purchase with spaces in merchant name",
			sms:                          "From HSBC: 26AUG25 PAYMOB-*2S FOR TRADI Purchase from 040-206***-001 EGP 1,698.00- Your available balance is EGP 44,278.59",
			expectedCounterparty:         "PAYMOB-*2S FOR TRADI",
			expectedAmount:               1698.00,
			expectedAvailableBalance:     44278.59,
			expectedDate:                 "26Aug25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Fawry shoes purchase",
			sms:                          "From HSBC: 26AUG25 FAWRY*KWTSHY SHOES Purchase from 040-206***-001 EGP 1,160.00- Your available balance is EGP 43,118.59",
			expectedCounterparty:         "FAWRY*KWTSHY SHOES",
			expectedAmount:               1160.00,
			expectedAvailableBalance:     43118.59,
			expectedDate:                 "26Aug25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Seoudi market purchase with long merchant name",
			sms:                          "From HSBC: 23SEP25 SEOUDI MARKET CITY S Purchase from 040-206***-001 EGP 330.53- Your available balance is EGP 11,160.98",
			expectedCounterparty:         "SEOUDI MARKET CITY S",
			expectedAmount:               330.53,
			expectedAvailableBalance:     11160.98,
			expectedDate:                 "23Sep25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "De Backers purchase",
			sms:                          "From HSBC: 08OCT25 DE BACKERS Purchase from 040-206***-001 EGP 8,085.00- Your available balance is EGP 41,562.85",
			expectedCounterparty:         "DE BACKERS",
			expectedAmount:               8085.00,
			expectedAvailableBalance:     41562.85,
			expectedDate:                 "08Oct25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Paymob Smash Gym November purchase",
			sms:                          "From HSBC: 01NOV25 PAYMOB-*SMASH GYM Purchase from 040-206***-001 EGP 4,000.00- Your available balance is EGP 73,665.86",
			expectedCounterparty:         "PAYMOB-*SMASH GYM",
			expectedAmount:               4000.00,
			expectedAvailableBalance:     73665.86,
			expectedDate:                 "01Nov25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
		},
		{
			name:                         "Numeric merchant ID",
			sms:                          "From HSBC: 02NOV25 1980 Purchase from 040-206***-001 EGP 1,181.60- Your available balance is EGP 69,180.96",
			expectedCounterparty:         "1980",
			expectedAmount:               1181.60,
			expectedAvailableBalance:     69180.96,
			expectedDate:                 "02Nov25",
			expectedAccountID:            "040-206***-001",
			expectedCurrency:             "EGP",
			expectedAvailableBalanceCurr: "EGP",
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
			sms:  "From HSBC: 02NOV25 1980 Purchase from 040-206***-001 EGP 1,181.60- Your available balance is EGP 69,180.96",
			want: true,
		},
		{
			name: "HSBC deposit (not purchase)",
			sms:  "From HSBC: 02NOV25 Deposit to 040-206***-001 EGP 5,000.00+ Your available balance is EGP 75,000.00",
			want: false,
		},
		{
			name: "Different bank",
			sms:  "From CIB: 02NOV25 Purchase EGP 1,000.00",
			want: false,
		},
		{
			name: "Missing 'Purchase from'",
			sms:  "From HSBC: 02NOV25 1980 EGP 1,181.60- Your available balance is EGP 69,180.96",
			want: false,
		},
		{
			name: "Empty string",
			sms:  "",
			want: false,
		},
		{
			name: "HSBC but no prefix",
			sms:  "HSBC: 02NOV25 1980 Purchase from 040-206***-001 EGP 1,181.60-",
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
			sms:     "From HSBC: 02NOV25 1980 Purchase from 040-206***-001 EGP 1,181.60- Your available balance is EGP 69,180.96",
			wantErr: false,
		},
		{
			name:    "Invalid date format returns error",
			sms:     "From HSBC: 99ZZZ99 1980 Purchase from 040-206***-001 EGP 1,181.60- Your available balance is EGP 69,180.96",
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
