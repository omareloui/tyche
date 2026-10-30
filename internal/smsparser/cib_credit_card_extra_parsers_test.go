package smsparser_test

import (
	"errors"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func TestCIBCreditCardExtraParsers(t *testing.T) {
	tests := []struct {
		name   string
		parser smsparser.Parser
		sms    string
		want   smsparser.SMS
	}{
		{
			name:   "charge without ending with",
			parser: &smsparser.CIBEnglishCreditCardChargeParser{},
			sms:    "Your credit card #5678 was charged for EGP 1,728.29 at Generic Store ABC on 25/10/26  at 10:00. Available limit is  78151.20 and the available international limit to use is EGP 135898",
			want: smsparser.SMS{
				Bank: smsparser.CIB, TxKind: models.ExpenseTxKind, AccountID: "5678",
				Currency: "EGP", Amount: 1728.29, Counterparty: "Generic Store ABC", AutoCreateCounterparty: true, AvailableBalance: 78151.20,
				Timestamp: time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
			},
		},
		{
			name:   "charge in foreign currency",
			parser: &smsparser.CIBEnglishCreditCardChargeParser{},
			sms:    "Your credit card #5678 was charged for USD 1.00 at ONLINE.COM on 24/10/26  at 13:32. Available limit is  78190.20 and the available international limit to use is EGP 135472",
			want: smsparser.SMS{
				Bank: smsparser.CIB, TxKind: models.ExpenseTxKind, AccountID: "5678",
				Currency: "USD", Amount: 1, Counterparty: "ONLINE.COM", AutoCreateCounterparty: true, AvailableBalance: 78190.20,
				Timestamp: time.Date(2026, 10, 24, 13, 32, 0, 0, time.UTC),
			},
		},
		{
			name:   "reversed transaction",
			parser: &smsparser.CIBEnglishCreditCardReversalParser{},
			sms:    "The transaction on your credit card#5678  from Ride Service  with EGP 247.00 on 21/10/26  at 18:04 has been refunded. Please try again. Thank you",
			want: smsparser.SMS{
				Bank: smsparser.CIB, TxKind: models.RefundTxKind, AccountID: "5678",
				Currency: "EGP", Amount: 247, Counterparty: "Ride Service", AutoCreateCounterparty: true,
				Timestamp: time.Date(2026, 10, 21, 18, 4, 0, 0, time.UTC),
			},
		},
		{
			name:   "arabic refund",
			parser: &smsparser.CIBArabicCreditCardRefundParser{},
			sms:    "لقد تم رد EGP3227.02 على بطاقتكم الائتمانية المنتهية بـ# 5678 من Online Shop>  . يرجى ملاحظة أن هذا المبلغ سيتم إضافته إلى رصيد بطاقتك ولا يتم اعتباره بمثابة دفعة للمديونيات المستحقة لهذا الشهر.",
			want: smsparser.SMS{
				Bank: smsparser.CIB, TxKind: models.RefundTxKind, AccountID: "5678",
				Currency: "EGP", Amount: 3227.02, Counterparty: "Online Shop", AutoCreateCounterparty: true,
			},
		},
		{
			name:   "arabic refund in foreign currency",
			parser: &smsparser.CIBArabicCreditCardRefundParser{},
			sms:    "لقد تم رد USD14.99 على بطاقتكم الائتمانية المنتهية بـ# 5678 من STREAMING SVC>  . يرجى ملاحظة أن هذا المبلغ سيتم إضافته إلى رصيد بطاقتك",
			want: smsparser.SMS{
				Bank: smsparser.CIB, TxKind: models.RefundTxKind, AccountID: "5678",
				Currency: "USD", Amount: 14.99, Counterparty: "STREAMING SVC", AutoCreateCounterparty: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.parser.CanParse(tt.sms) {
				t.Fatal("CanParse() = false, want true")
			}
			got, err := tt.parser.Parse(tt.sms)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if *got != tt.want {
				t.Errorf("Parse() =\n%+v\nwant\n%+v", *got, tt.want)
			}
		})
	}
}

func TestCIBExtraParsersThroughRegistry(t *testing.T) {
	r := smsparser.Default()

	informational := []string{
		"تم استلام الطلب لرد مبلغ من Online Shop بقيمة EGP -320.00 وسيتم اضافته للبطاقة الائتمانية # 5678 بمجرد استلام المبلغ من التاجر. للمزيد من التفاصيل http://cib.eg/mb",
		"عميلنا العزيز، سيتم خصم 19694.80 جم من حسابكم لسداد بطاقتك المنتهية بـ 5678  في 05/10, في حالة أن يوم السداد يزامن عطلة رسمية سوف يتم الخصم قبل العطلات الرسمية.",
		"كشف حساب شهر أكتوبر 2026  لبطاقتك الائتمانية المنتهية بـ 5678",
	}
	for _, sms := range informational {
		if _, err := r.ParseFromSender("CIB", sms); !errors.Is(err, smsparser.ErrNotTransaction) {
			t.Errorf("expected ErrNotTransaction for %q, got %v", sms, err)
		}
	}

	// the older format with "ending with" still goes to its own parser
	got, err := r.ParseFromSender("CIB", "Your credit card ending with#5678 was charged for EGP 150.00 at Generic Store ABC on 04/12/25  at 10:48. Card available limit is EGP  95000.00. For more details, please visit https://cib.eg/mb")
	if err != nil || got.AccountID != "5678" || got.AvailableBalanceCurrency != "EGP" {
		t.Errorf("ending with format: got %+v, %v", got, err)
	}
}
