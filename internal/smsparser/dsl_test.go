package smsparser_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCompileAndMatch(t *testing.T) {
	tests := []struct {
		name, body, sms string
		want            map[string]string // nil for no match
	}{
		{
			name: "whitespace runs match any whitespace",
			body: "Paid {{amount}} on {{date}} at {{time}}",
			sms:  "Paid   10.00 on 13/05/26  at 10:21",
			want: map[string]string{"amount": "10.00", "date": "13/05/26", "time": "10:21"},
		},
		{
			name: "placeholders ignore the whitespace around them",
			body: "card ending with{{account_id}} for {{currency}}{{amount}}",
			sms:  "card ending with #9545 for EGP 917.95",
			want: map[string]string{"account_id": "#9545", "currency": "EGP", "amount": "917.95"},
		},
		{
			name: "the whole sms has to match",
			body: "Paid {{amount}}",
			sms:  "Paid 10.00 at Shop",
		},
		{
			name: "{{*}} skips any text",
			body: "Paid {{amount}}{{*}}",
			sms:  "Paid 10.00 at Shop.\nThanks",
			want: map[string]string{"amount": "10.00"},
		},
		{
			name: "anonymous regexp",
			body: `Code {{/\d{6}/}} for {{counterparty}}`,
			sms:  "Code 123456 for Some Shop",
			want: map[string]string{"counterparty": "Some Shop"},
		},
		{
			name: "anonymous regexp has to match",
			body: `Code {{/\d{6}/}} for {{counterparty}}`,
			sms:  "Code 12 for Some Shop",
		},
		{
			name: "field with a regexp",
			body: `Ref {{ref:/[A-Z]{2}\d+/}} done`,
			sms:  "Ref AB123 done",
			want: map[string]string{"ref": "AB123"},
		},
		{
			name: "literal regexp characters are escaped",
			body: "Total (incl. VAT) {{amount}}*",
			sms:  "Total (incl. VAT) 1,000*",
			want: map[string]string{"amount": "1,000"},
		},
		{
			name: "values have their whitespace collapsed",
			body: "from {{counterparty}} on",
			sms:  "from  JOHN   DOE on",
			want: map[string]string{"counterparty": "JOHN DOE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := smsparser.Compile(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := p.Match(tt.sms)
			if tt.want == nil {
				if ok {
					t.Fatalf("matched %v, want no match", got)
				}
				return
			}
			if !ok {
				t.Fatal("didn't match")
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"", "empty template"},
		{"Paid {{amount", "unclosed {{"},
		{"Paid {{amount}} {{amount}}", "more than once"},
		{"Paid {{nope}}", `unknown field "nope"`},
		{"Paid {{amount:big}}", `unknown modifier "big"`},
		{"Paid {{/(/}}", "invalid regexp"},
		{"Paid {{amount:/[/}}", "invalid regexp"},
		{"On {{date:YYYY}}", "needs a day"},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			_, err := smsparser.Compile(tt.body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestDates(t *testing.T) {
	tests := []struct{ body, sms, want string }{
		{"on {{date}}", "on 13/05/26", "2026-05-13 00:00"},
		{"on {{date}}", "on 26/10/2026", "2026-10-26 00:00"},
		{"on {{date}}", "on 7-11-2025", "2025-11-07 00:00"},
		{"on {{date}}", "on 07OCT25", "2025-10-07 00:00"},
		{"on {{date}}", "on 07 Dec 2025", "2025-12-07 00:00"},
		{"on {{date}} {{time}}", "on 07-11-2025 16:54", "2025-11-07 16:54"},
		{"on {{date}} {{time}}", "on 07-11-2025 9:05 pm", "2025-11-07 21:05"},
		{"on {{date}} {{time}}", "on 07-11-2025 21:05:30", "2025-11-07 21:05"},
		{"on {{date:MM/DD/YYYY}}", "on 05/13/2026", "2026-05-13 00:00"},
		{"on {{date:YYYY-MM-DD}}", "on 2026-05-13", "2026-05-13 00:00"},
		{"on {{date:DD/MM}}", "on 13/05", "0000-05-13 00:00"},
		// matches but isn't a date, the sms is still read
		{"on {{date}}", "on 99ZZZ99", ""},
	}
	for _, tt := range tests {
		t.Run(tt.sms, func(t *testing.T) {
			tmpl := models.SMSTemplate{
				Body: tt.body + " paid {{amount}}", IsEnabled: true,
				Action: models.SMSTemplateTransaction, TxKind: models.ExpenseTxKind,
			}
			m, err := smsparser.MatchTemplate(tmpl, tt.sms+" paid 10")
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if !m.SMS.Timestamp.IsZero() {
				got = m.SMS.Timestamp.Format("2006-01-02 15:04")
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if wantTime := strings.Contains(tt.body, "{{time}}"); m.SMS.HasTime != wantTime {
				t.Errorf("HasTime = %v, want %v", m.SMS.HasTime, wantTime)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	tmpl := models.SMSTemplate{
		Body:      "{{amount}} {{currency}} {{counterparty}} {{to_account_id}}",
		IsEnabled: true, Action: models.SMSTemplateTransaction, TxKind: models.TransferTxKind,
		Fixed: map[string]string{"account_id": "Cash", "counterparty": "Overridden", "note": "ATM"},
	}
	m, err := smsparser.MatchTemplate(tmpl, "2 111.00+ جم Shop> **1234")
	if err != nil {
		t.Fatal(err)
	}
	want := smsparser.SMS{
		TxKind: models.TransferTxKind, AccountID: "Cash", ToAccountID: "1234",
		Amount: 2111, ToAmount: 2111, Currency: "EGP", ToCurrency: "EGP",
		Counterparty: "Shop", AutoCreateCounterparty: true, Note: "ATM",
	}
	if *m.SMS != want {
		t.Errorf("got\n%+v\nwant\n%+v", *m.SMS, want)
	}
}
