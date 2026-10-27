package smsparser

import (
	"regexp"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

// ArabBankArabicSalaryParser parses the salary credit message, the body
// starts with a country code line:
// EG
// قيد راتب بمبلغ USD 1000.00 لحساب 123*4567 بتاريخ 26/10/2026 .
type ArabBankArabicSalaryParser struct{}

var arabBankSalaryRe = regexp.MustCompile(
	`قيد راتب بمبلغ\s+([A-Z]{3})\s+([\d,.]+)\s+لحساب\s+(\S+)\s+بتاريخ\s+(\d{2}/\d{2}/\d{4})`)

func (p *ArabBankArabicSalaryParser) CanParse(sms string) bool {
	return arabBankSalaryRe.MatchString(sms)
}

func (p *ArabBankArabicSalaryParser) Parse(sms string) (*SMS, error) {
	m := arabBankSalaryRe.FindStringSubmatch(sms)
	if m == nil {
		return nil, ErrCantParse
	}

	amount, err := amountStrToFloat(m[2])
	if err != nil {
		return nil, err
	}

	result := &SMS{
		Bank:      ArabBank,
		TxKind:    models.IncomeTxKind,
		Currency:  m[1],
		Amount:    amount,
		AccountID: m[3],
	}
	if timestamp, err := time.Parse("02/01/2006", m[4]); err == nil {
		result.Timestamp = timestamp
	}

	return result, nil
}
