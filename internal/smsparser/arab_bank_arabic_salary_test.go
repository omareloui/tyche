package smsparser_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestArabBankArabicSalaryParser(t *testing.T) {
	parser := &smsparser.ArabBankArabicSalaryParser{}

	tests := []struct {
		name    string
		sms     string
		want    *smsparser.SMS
		wantErr bool
	}{
		{
			name: "with country code line",
			sms:  "EG\n قيد راتب بمبلغ USD 1000.50 لحساب 123*4567 بتاريخ 26/10/2026 .",
			want: &smsparser.SMS{
				Bank:      smsparser.ArabBank,
				TxKind:    models.IncomeTxKind,
				AccountID: "123*4567",
				Currency:  "USD",
				Amount:    1000.50,
				Timestamp: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "without country code and thousands separator",
			sms:  "قيد راتب بمبلغ EGP 25,000.00 لحساب 987*6543 بتاريخ 01/02/2026 .",
			want: &smsparser.SMS{
				Bank:      smsparser.ArabBank,
				TxKind:    models.IncomeTxKind,
				AccountID: "987*6543",
				Currency:  "EGP",
				Amount:    25000,
				Timestamp: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "different message",
			sms:     "EG\n تم خصم مبلغ USD 10.00 من حساب 123*4567 بتاريخ 26/10/2026 .",
			wantErr: true,
		},
		{
			name:    "empty",
			sms:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parser.CanParse(tt.sms); got == tt.wantErr {
				t.Errorf("CanParse() = %v, want %v", got, !tt.wantErr)
			}

			got, err := parser.Parse(tt.sms)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if *got != *tt.want {
				t.Errorf("Parse() = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}
