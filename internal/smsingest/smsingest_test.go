package smsingest_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsingest"
	"github.com/omareloui/tyche/internal/smsparser"
)

var (
	egp = models.Currency{ID: "egp", String: "EGP", Symbol: "E£"}
	usd = models.Currency{ID: "usd", String: "USD", Symbol: "US$"}

	hsbcEGP = models.Account{ID: "hsbc-egp", Currency: egp, Kind: models.AccountKind{Enum: models.AccKindBankAcc}, SmsRefs: []string{"123-456***-789", "9876"}}
	hsbcUSD = models.Account{ID: "hsbc-usd", Currency: usd, Kind: models.AccountKind{Enum: models.AccKindBankAcc}, SmsRefs: []string{"123-456***-790"}}
	cibCard = models.Account{ID: "cib-card", Currency: egp, Kind: models.AccountKind{Enum: models.AccKindCreditCard}, CardNumber: "4111111111111234"}
	cash    = models.Account{ID: "cash", Currency: egp, Kind: models.AccountKind{Enum: models.AccKindCash}}
	oldCard = models.Account{ID: "old", Currency: egp, Kind: models.AccountKind{Enum: models.AccKindCreditCard}, CardNumber: "5500000000005678", IsArchive: true}

	accounts       = []models.Account{hsbcEGP, hsbcUSD, cibCard, cash, oldCard}
	counterparties = []models.Counterparty{{ID: "cp-uber", Name: "Ride Service A", Aliases: []string{"RIDE*TRIP", "Ride BV"}}}

	receivedAt = time.Date(2025, 10, 7, 13, 30, 0, 0, time.UTC)
)

func fx(from, to string) (float64, error) {
	if from == "USD" && to == "EGP" {
		return 50, nil
	}
	return 0, errors.New("no rate")
}

func TestBuildTransaction(t *testing.T) {
	tests := []struct {
		name        string
		sms         smsparser.SMS
		wantErr     bool
		wantKind    models.TxKind
		wantAcc     string
		wantToAcc   string
		wantAmount  float64
		wantToAmt   float64
		wantCp      string
		wantNewCp   string
		wantDescrip string
		wantMerch   bool
	}{
		{
			name:        "expense by sms ref",
			sms:         smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "123-456***-789", Currency: "EGP", Amount: 220, Counterparty: "PAYPRO-*SHOP"},
			wantKind:    models.ExpenseTxKind,
			wantAcc:     "hsbc-egp",
			wantAmount:  220,
			wantDescrip: "PAYPRO-*SHOP",
		},
		{
			name:       "expense by the account's other sms ref",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "9876", Currency: "EGP", Amount: 75},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "hsbc-egp",
			wantAmount: 75,
		},
		{
			name:       "other sms ref by its last digits",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "****9876", Currency: "EGP", Amount: 75},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "hsbc-egp",
			wantAmount: 75,
		},
		{
			name:       "card last digits and known counterparty",
			sms:        smsparser.SMS{TxKind: models.IncomeTxKind, AccountID: "**1234", Currency: "EGP", Amount: 45.2, Counterparty: "ride service a"},
			wantKind:   models.IncomeTxKind,
			wantAcc:    "cib-card",
			wantAmount: 45.2,
			wantCp:     "cp-uber",
		},
		{
			name:       "auto creates unmatched counterparty",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "1234", Currency: "EGP", Amount: 99, Counterparty: " Food App ", AutoCreateCounterparty: true},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "cib-card",
			wantAmount: 99,
			wantNewCp:  "Food App",
		},
		{
			name:       "auto creates merchants",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "1234", Currency: "EGP", Amount: 99, Counterparty: "Food App", AutoCreateCounterparty: true, CounterpartyIsMerchant: true},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "cib-card",
			wantAmount: 99,
			wantNewCp:  "Food App",
			wantMerch:  true,
		},
		{
			name:        "note goes first in the description",
			sms:         smsparser.SMS{TxKind: models.TransferTxKind, AccountID: "123-456***-789", ToAccountID: "EG12345", Currency: "EGP", Amount: 360, Note: "CIB BRANCH A ATM"},
			wantKind:    models.ExpenseTxKind,
			wantAcc:     "hsbc-egp",
			wantAmount:  360,
			wantDescrip: "CIB BRANCH A ATM · Transfer to EG12345",
		},
		{
			name:       "auto create prefers existing counterparty",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "1234", Currency: "EGP", Amount: 99, Counterparty: "RIDE BV", AutoCreateCounterparty: true},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "cib-card",
			wantAmount: 99,
			wantCp:     "cp-uber",
		},
		{
			name:       "counterparty matched by alias case insensitively",
			sms:        smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "**1234", Currency: "EGP", Amount: 80, Counterparty: " ride*trip "},
			wantKind:   models.ExpenseTxKind,
			wantAcc:    "cib-card",
			wantAmount: 80,
			wantCp:     "cp-uber",
		},
		{
			name:        "converts foreign currency to account currency",
			sms:         smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "1234", Currency: "USD", Amount: 10, Counterparty: "Netflix"},
			wantKind:    models.ExpenseTxKind,
			wantAcc:     "cib-card",
			wantAmount:  500,
			wantDescrip: "Netflix · originally USD 10",
		},
		{
			name:    "missing fx rate fails",
			sms:     smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "1234", Currency: "EUR", Amount: 10},
			wantErr: true,
		},
		{
			name:       "atm withdrawal to cash",
			sms:        smsparser.SMS{TxKind: models.TransferTxKind, AccountID: "**1234", ToAccountID: "Cash", Currency: "EGP", Amount: 1000},
			wantKind:   models.TransferTxKind,
			wantAcc:    "cib-card",
			wantToAcc:  "cash",
			wantAmount: 1000,
			wantToAmt:  1000,
		},
		{
			name:        "transfer to unknown account becomes expense",
			sms:         smsparser.SMS{TxKind: models.TransferTxKind, AccountID: "123-456***-789", ToAccountID: "EG12345", Currency: "EGP", Amount: 360},
			wantKind:    models.ExpenseTxKind,
			wantAcc:     "hsbc-egp",
			wantAmount:  360,
			wantDescrip: "Transfer to EG12345",
		},
		{
			name:        "transfer from unknown account becomes income",
			sms:         smsparser.SMS{TxKind: models.TransferTxKind, AccountID: "999***111", ToAccountID: "123-456***-790", Currency: "USD", Amount: 5},
			wantKind:    models.IncomeTxKind,
			wantAcc:     "hsbc-usd",
			wantAmount:  5,
			wantDescrip: "Transfer from 999***111",
		},
		{
			name:    "archived accounts are ignored",
			sms:     smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "**5678", Currency: "EGP", Amount: 1},
			wantErr: true,
		},
		{
			name:    "too few digits to match",
			sms:     smsparser.SMS{TxKind: models.ExpenseTxKind, AccountID: "**34", Currency: "EGP", Amount: 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := smsingest.BuildTransaction(&tt.sms, accounts, counterparties, receivedAt, time.UTC, fx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", tx)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tx.Kind != tt.wantKind {
				t.Errorf("kind = %v, want %v", tx.Kind, tt.wantKind)
			}
			if tx.Account.ID != tt.wantAcc {
				t.Errorf("account = %q, want %q", tx.Account.ID, tt.wantAcc)
			}
			var toAcc string
			if tx.ToAccount != nil {
				toAcc = tx.ToAccount.ID
			}
			if toAcc != tt.wantToAcc {
				t.Errorf("to account = %q, want %q", toAcc, tt.wantToAcc)
			}
			if math.Abs(tx.Amount-tt.wantAmount) > 1e-9 {
				t.Errorf("amount = %v, want %v", tx.Amount, tt.wantAmount)
			}
			if math.Abs(tx.ToAmount-tt.wantToAmt) > 1e-9 {
				t.Errorf("to amount = %v, want %v", tx.ToAmount, tt.wantToAmt)
			}
			var cp, newCp string
			if tx.Counterparty != nil {
				cp = tx.Counterparty.ID
				if cp == "" {
					newCp = tx.Counterparty.Name
				}
			}
			if cp != tt.wantCp {
				t.Errorf("counterparty = %q, want %q", cp, tt.wantCp)
			}
			if newCp != tt.wantNewCp {
				t.Errorf("new counterparty = %q, want %q", newCp, tt.wantNewCp)
			}
			if newCp != "" && tx.Counterparty.IsMerchant != tt.wantMerch {
				t.Errorf("new counterparty merchant = %v, want %v", tx.Counterparty.IsMerchant, tt.wantMerch)
			}
			if tx.Description != tt.wantDescrip {
				t.Errorf("description = %q, want %q", tx.Description, tt.wantDescrip)
			}
			if !tx.NeedsReview || !tx.FulfilledAt.Equal(receivedAt) || !tx.ScheduledTo.Equal(receivedAt) {
				t.Errorf("expected fulfilled, scheduled at received time and needing review, got %+v", tx)
			}
		})
	}
}

func TestBuildTransactionDefaultCategory(t *testing.T) {
	transport := &models.Category{ID: "cat-transport", Name: "Transport", Kinds: []models.TxKind{models.ExpenseTxKind}}
	cps := []models.Counterparty{{ID: "cp-uber", Name: "Ride Service A", DefaultCategory: transport}}

	tests := []struct {
		name    string
		kind    models.TxKind
		wantCat string
	}{
		{name: "matching kind", kind: models.ExpenseTxKind, wantCat: "cat-transport"},
		{name: "other kind", kind: models.IncomeTxKind, wantCat: ""},
		{name: "refund uses the expense category", kind: models.RefundTxKind, wantCat: "cat-transport"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sms := smsparser.SMS{TxKind: tt.kind, AccountID: "123-456***-789", Currency: "EGP", Amount: 100, Counterparty: "Ride Service A"}
			tx, err := smsingest.BuildTransaction(&sms, accounts, cps, receivedAt, time.UTC, fx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var cat string
			if tx.Category != nil {
				cat = tx.Category.ID
			}
			if cat != tt.wantCat {
				t.Errorf("category = %q, want %q", cat, tt.wantCat)
			}
		})
	}
}

func TestTxTime(t *testing.T) {
	cairo, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatal(err)
	}
	// 2025-10-07 15:30 in Cairo (UTC+3 then)
	received := time.Date(2025, 10, 7, 12, 30, 0, 0, time.UTC)
	date := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, time.UTC) }

	tests := []struct {
		name string
		sms  smsparser.SMS
		want time.Time
	}{
		{"no date", smsparser.SMS{}, received},
		{"date and time in the sms time zone", smsparser.SMS{Timestamp: date(2025, 10, 6, 21, 45), HasTime: true}, time.Date(2025, 10, 6, 21, 45, 0, 0, cairo)},
		{"date only gets the received time", smsparser.SMS{Timestamp: date(2025, 10, 5, 0, 0)}, time.Date(2025, 10, 5, 15, 30, 0, 0, cairo)},
		{"same day keeps the received time", smsparser.SMS{Timestamp: date(2025, 10, 7, 0, 0)}, received},
		{"future date falls back", smsparser.SMS{Timestamp: date(2025, 10, 9, 10, 0), HasTime: true}, received},
		{"slightly ahead clock is kept", smsparser.SMS{Timestamp: date(2025, 10, 7, 15, 50), HasTime: true}, time.Date(2025, 10, 7, 15, 50, 0, 0, cairo)},
		{"no year is this year", smsparser.SMS{Timestamp: date(0, 9, 1, 10, 0), HasTime: true}, time.Date(2025, 9, 1, 10, 0, 0, 0, cairo)},
		{"no year in the future is last year", smsparser.SMS{Timestamp: date(0, 12, 30, 10, 0), HasTime: true}, time.Date(2024, 12, 30, 10, 0, 0, 0, cairo)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smsingest.TxTime(&tt.sms, received, cairo); !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHash(t *testing.T) {
	a := smsingest.Hash("HSBC", "body", receivedAt)
	if a != smsingest.Hash(" HSBC", "body\n", receivedAt) {
		t.Error("hash should ignore surrounding whitespace")
	}
	if a == smsingest.Hash("HSBC", "body", receivedAt.Add(time.Second)) {
		t.Error("hash should depend on received time")
	}
}
