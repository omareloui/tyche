package smsparser_test

import (
	"testing"

	"github.com/omareloui/tyche/internal/smsparser"
)

func TestBankFromSender(t *testing.T) {
	tests := []struct {
		sender string
		want   smsparser.Bank
	}{
		{"HSBC", smsparser.HSBC},
		{" cib ", smsparser.CIB},
		{"AAIB", smsparser.AAIB},
		{"Arab Bank", smsparser.ArabBank},
		{"+201000000000", smsparser.NilBank},
	}
	for _, tt := range tests {
		if got := smsparser.BankFromSender(tt.sender); got != tt.want {
			t.Errorf("BankFromSender(%q) = %v, want %v", tt.sender, got, tt.want)
		}
	}
}

func TestDefaultRegistry_ParseFromSender(t *testing.T) {
	r := smsparser.Default()

	tests := []struct {
		name     string
		sender   string
		sms      string
		wantBank smsparser.Bank
		wantErr  bool
	}{
		{
			name:     "known sender",
			sender:   "HSBC",
			sms:      "From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85",
			wantBank: smsparser.HSBC,
		},
		{
			name:     "unknown sender falls back to all banks",
			sender:   "19666",
			sms:      "تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP",
			wantBank: smsparser.CIB,
		},
		{
			name:     "trims surrounding whitespace",
			sender:   "AAIB",
			sms:      "\nYour credit card **5678 was debited by 229.00EGP at Generic Store ABC. Available limit is 100000.00. ",
			wantBank: smsparser.AAIB,
		},
		{
			name:     "arab bank sender with spaces",
			sender:   "Arab Bank",
			sms:      "EG\n قيد راتب بمبلغ USD 1000.00 لحساب 123*4567 بتاريخ 26/10/2026 .",
			wantBank: smsparser.ArabBank,
		},
		{
			name:    "known sender, unknown format",
			sender:  "HSBC",
			sms:     "Your OTP is 123456",
			wantErr: true,
		},
		{
			name:    "unknown sender, unknown format",
			sender:  "Vodafone",
			sms:     "Your bill is ready",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.ParseFromSender(tt.sender, tt.sms)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Bank != tt.wantBank {
				t.Errorf("bank = %v, want %v", got.Bank, tt.wantBank)
			}
		})
	}
}
