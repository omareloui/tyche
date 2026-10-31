package smsparser_test

import (
	"errors"
	"testing"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestSeedsCompileAndMatchTheirSample(t *testing.T) {
	keys := map[string]bool{}
	for _, s := range smsparser.Seeds() {
		if keys[s.SeedKey] {
			t.Errorf("duplicate seed key %q", s.SeedKey)
		}
		keys[s.SeedKey] = true

		m, err := smsparser.MatchTemplate(s, s.Sample)
		if s.Action == models.SMSTemplateIgnore {
			if !errors.Is(err, smsparser.ErrNotTransaction) {
				t.Errorf("%s: sample got %v, want ignored", s.SeedKey, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: sample didn't match: %v", s.SeedKey, err)
			continue
		}
		if m.SMS.Amount == 0 || (m.SMS.AccountID == "" && m.SMS.ToAccountID == "") {
			t.Errorf("%s: sample read without amount or account: %+v", s.SeedKey, m.SMS)
		}
	}
}

func TestSeeds(t *testing.T) {
	r := smsparser.NewRegistry(smsparser.Seeds())
	if errs := r.Errors(); len(errs) > 0 {
		t.Fatalf("seeds didn't compile: %v", errs)
	}

	cases := append(seedCases,
		seedCase{
			sender: "CIB",
			sms:    "Your credit card ending with#9545 was charged for EGP 235.62 at Uber on 13/05/26  at 10:21. Card available limit is EGP  67766.74. For more details, please visit https://cib.eg/mb",
			key:    "cib.en.cc_purchase",
			want: smsparser.SMS{
				TxKind: expense, AccountID: "9545", Currency: "EGP", Amount: 235.62, Counterparty: "Uber",
				AutoCreateCounterparty: true, CounterpartyIsMerchant: true, AvailableBalance: 67766.74, AvailableBalanceCurrency: "EGP",
			},
			ts: "2026-05-13 10:21",
		},
		seedCase{
			sender: "CIB",
			sms:    "لقد تم رفض المعاملة من Vox Cinemas على بطاقتكم الائتمانية المنتهية بـ9545 بقيمة 235.00 EGP نظراً لوجود خطأ برمز التحقق المدون خلف البطاقة. يرجى إعادة إدخال رمز التحقق الصحيح.",
			key:    "-",
		},
		seedCase{
			sender: "AAIB",
			sms:    "Your transaction on credit card **1432 for 189.99 EGP at GOOGLE*TELEGRAM was declined due to Insufficient balance. Your available limit is 7.4.",
			key:    "-",
		},
		seedCase{
			sender: "AAIB",
			sms:    "Your credit card **1432 was debited by 240.00EGP at Netflix.com. Available limit is 4 098.89.",
			key:    "aaib.en.cc_purchase",
			want: smsparser.SMS{
				TxKind: expense, AccountID: "1432", Currency: "EGP", Amount: 240, Counterparty: "Netflix.com",
				AutoCreateCounterparty: true, CounterpartyIsMerchant: true, AvailableBalance: 4098.89,
			},
		},
		seedCase{
			sender: "AAIB",
			sms:    "Credit Card Payment of EGP 1934.84 has been debited from your account 1001-01 through AAIB Mobile, to your credit card ends with 1432",
			key:    "aaib.en.cc_payment",
			want: smsparser.SMS{
				TxKind: transfer, AccountID: "1001-01", ToAccountID: "1432", Currency: "EGP", ToCurrency: "EGP", Amount: 1934.84, ToAmount: 1934.84,
			},
		},
		seedCase{
			sender: "AAIB",
			sms:    "Dear Valued customer, your AAIB card **1432 has been credited with 1 934.84EGP on cairo your available balance is 4 496.87.",
			key:    "-",
		},
	)

	for _, tt := range cases {
		t.Run(tt.sms, func(t *testing.T) {
			m, err := r.Parse(tt.sender, tt.sms)
			switch tt.key {
			case "":
				if err == nil {
					t.Fatalf("expected no match, got %q: %+v", m.Template.SeedKey, m.SMS)
				}
				if !errors.Is(err, smsparser.ErrCantParse) {
					t.Fatalf("err = %v, want ErrCantParse", err)
				}
				return
			case "-":
				if !errors.Is(err, smsparser.ErrNotTransaction) {
					t.Fatalf("err = %v, want ErrNotTransaction", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Template.SeedKey != tt.key {
				t.Errorf("template = %q, want %q", m.Template.SeedKey, tt.key)
			}

			got := *m.SMS
			ts := ""
			if !got.Timestamp.IsZero() {
				ts = got.Timestamp.Format("2006-01-02 15:04")
			}
			if ts != tt.ts {
				t.Errorf("timestamp = %q, want %q", ts, tt.ts)
			}
			got.Bank, got.Timestamp, got.HasTime = "", tt.want.Timestamp, false
			if got != tt.want {
				t.Errorf("got\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestParseSenders(t *testing.T) {
	r := smsparser.NewRegistry(smsparser.Seeds())
	const hsbcTransfer = "From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85"

	tests := []struct {
		name, sender, sms, wantBank string
	}{
		{"known sender", "HSBC", hsbcTransfer, "HSBC"},
		{"sender is case-insensitive and trimmed", " hsbc ", hsbcTransfer, "HSBC"},
		{"unknown sender tries every template", "19666", hsbcTransfer, "HSBC"},
		{"arab bank sender with spaces", "Arab Bank", "EG\n قيد راتب بمبلغ USD 1000.00 لحساب 123*4567 بتاريخ 26/10/2026 .", "Arab Bank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := r.Parse(tt.sender, tt.sms)
			if err != nil {
				t.Fatal(err)
			}
			if m.SMS.Bank != tt.wantBank {
				t.Errorf("bank = %q, want %q", m.SMS.Bank, tt.wantBank)
			}
		})
	}

	if _, err := r.Parse("CIB", hsbcTransfer); err == nil {
		t.Error("an sms from a known sender matched another bank's template")
	}
}

func TestRegistryOrder(t *testing.T) {
	const sms = "Paid EGP 10.00 at Shop"
	tmpl := func(id, body string, priority int) models.SMSTemplate {
		return models.SMSTemplate{ID: id, Body: body, Priority: priority, IsEnabled: true, Action: models.SMSTemplateTransaction, TxKind: expense}
	}
	generic := tmpl("generic", "Paid {{currency}} {{amount}} at {{*}}", 0)
	specific := tmpl("specific", "Paid {{currency}} {{amount}} at {{counterparty}}", 1)
	disabled := tmpl("disabled", "Paid {{*}}", 9)
	disabled.IsEnabled = false

	m, err := smsparser.NewRegistry([]models.SMSTemplate{generic, specific, disabled}).Parse("", sms)
	if err != nil || m.Template.ID != "specific" {
		t.Errorf("higher priority first: got %v, %v", m, err)
	}

	m, err = smsparser.NewRegistry([]models.SMSTemplate{generic}, []models.SMSTemplate{specific}).Parse("", sms)
	if err != nil || m.Template.ID != "generic" {
		t.Errorf("earlier group first: got %v, %v", m, err)
	}

	r := smsparser.NewRegistry([]models.SMSTemplate{tmpl("broken", "Paid {{nope}}", 0), specific})
	if len(r.Errors()) != 1 {
		t.Errorf("errors = %v, want the broken template", r.Errors())
	}
	if m, err := r.Parse("", sms); err != nil || m.Template.ID != "specific" {
		t.Errorf("broken templates are skipped: got %v, %v", m, err)
	}
}
