package smsparser

import (
	"regexp"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

// CIBEnglishCreditCardChargeParser parses the purchase format without
// "ending with":
// Your credit card #5678 was charged for EGP 10.00 at Google YouTube on 25/10/26  at 10:00. Available limit is  78151.20 and the available international limit to use is EGP 135898
type CIBEnglishCreditCardChargeParser struct{}

var cibCCChargeRe = regexp.MustCompile(
	`^Your credit card #(\d+) was charged for ([A-Z]{3}) ([\d,.]+) at (.+?) on (\d{2}/\d{2}/\d{2})\s+at (\d{2}:\d{2})\.\s+Available limit is\s+([\d,.]+)`)

func (p *CIBEnglishCreditCardChargeParser) CanParse(sms string) bool {
	return cibCCChargeRe.MatchString(sms)
}

func (p *CIBEnglishCreditCardChargeParser) Parse(sms string) (*SMS, error) {
	m := cibCCChargeRe.FindStringSubmatch(sms)
	if m == nil {
		return nil, ErrCantParse
	}

	amount, err := amountStrToFloat(m[3])
	if err != nil {
		return nil, err
	}
	available, err := amountStrToFloat(m[7])
	if err != nil {
		return nil, err
	}

	return &SMS{
		Bank:                   CIB,
		TxKind:                 models.ExpenseTxKind,
		AccountID:              m[1],
		Currency:               m[2],
		Amount:                 amount,
		Counterparty:           strings.TrimSpace(m[4]),
		AutoCreateCounterparty: true,
		AvailableBalance:       available,
		Timestamp:              parseCIBTime(m[5], m[6]),
	}, nil
}

// CIBEnglishCreditCardReversalParser parses a reversed card transaction,
// sent after its charge SMS so it's recorded as an income to cancel it out:
// The transaction on your credit card#5678  from DiDi  with EGP 247.00 on 21/10/26  at 18:04 has been refunded. Please try again. Thank you
type CIBEnglishCreditCardReversalParser struct{}

var cibCCReversalRe = regexp.MustCompile(
	`^The transaction on your credit card\s*#\s*(\d+)\s+from (.+?)\s+with ([A-Z]{3}) ([\d,.]+) on (\d{2}/\d{2}/\d{2})\s+at (\d{2}:\d{2}) has been refunded`)

func (p *CIBEnglishCreditCardReversalParser) CanParse(sms string) bool {
	return cibCCReversalRe.MatchString(sms)
}

func (p *CIBEnglishCreditCardReversalParser) Parse(sms string) (*SMS, error) {
	m := cibCCReversalRe.FindStringSubmatch(sms)
	if m == nil {
		return nil, ErrCantParse
	}

	amount, err := amountStrToFloat(m[4])
	if err != nil {
		return nil, err
	}

	return &SMS{
		Bank:                   CIB,
		TxKind:                 models.IncomeTxKind,
		AccountID:              m[1],
		Currency:               m[3],
		Amount:                 amount,
		Counterparty:           strings.TrimSpace(m[2]),
		AutoCreateCounterparty: true,
		Timestamp:              parseCIBTime(m[5], m[6]),
	}, nil
}

// CIBArabicCreditCardRefundParser parses a merchant refund added to the card:
// لقد تم رد EGP917.95 على بطاقتكم الائتمانية المنتهية بـ# 5678 من aliexpress>  . يرجى ملاحظة أن هذا المبلغ سيتم إضافته إلى رصيد بطاقتك ...
type CIBArabicCreditCardRefundParser struct{}

var cibCCRefundRe = regexp.MustCompile(
	`^لقد تم رد ([A-Z]{3})\s*([\d,.]+) على بطاقتكم الائتمانية المنتهية بـ\s*#\s*(\d+) من (.+?)>?\s*\.`)

func (p *CIBArabicCreditCardRefundParser) CanParse(sms string) bool {
	return cibCCRefundRe.MatchString(sms)
}

func (p *CIBArabicCreditCardRefundParser) Parse(sms string) (*SMS, error) {
	m := cibCCRefundRe.FindStringSubmatch(sms)
	if m == nil {
		return nil, ErrCantParse
	}

	amount, err := amountStrToFloat(m[2])
	if err != nil {
		return nil, err
	}

	return &SMS{
		Bank:                   CIB,
		TxKind:                 models.IncomeTxKind,
		AccountID:              m[3],
		Currency:               m[1],
		Amount:                 amount,
		Counterparty:           strings.TrimSpace(m[4]),
		AutoCreateCounterparty: true,
	}, nil
}

// CIBInformationalParser recognizes CIB messages that aren't transactions, so
// they get ignored instead of reported as unparsed.
type CIBInformationalParser struct{}

var cibInformationalPrefixes = []string{
	// refund request received, the refund itself comes in a later sms
	"تم استلام الطلب لرد مبلغ",
	// monthly credit card statement
	"كشف حساب شهر",
	// upcoming credit card auto payment reminder
	"عميلنا العزيز، سيتم خصم",
}

func (p *CIBInformationalParser) CanParse(sms string) bool {
	for _, prefix := range cibInformationalPrefixes {
		if strings.HasPrefix(sms, prefix) {
			return true
		}
	}
	return false
}

func (p *CIBInformationalParser) Parse(sms string) (*SMS, error) {
	return nil, ErrNotTransaction
}

func parseCIBTime(date, clock string) time.Time {
	t, _ := time.Parse("02/01/06 15:04", date+" "+clock)
	return t
}
