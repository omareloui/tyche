package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicRefundParser struct{}

func (p *CIBArabicRefundParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "تم اضافة المعاملة الخاصة بـ") &&
		strings.Contains(sms, "على بطاقتكم المنتهية بـ") &&
		strings.Contains(sms, "بقيمة")
}

func (p *CIBArabicRefundParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.IncomeTxKind}

	parts := strings.Fields(sms)

	// تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025
	// 0  1     2        3      4  5      6           7     8   10  11      12       13 14   15    16  17     18     19

	i := 5

	result.Counterparty = parts[i]
	i++
	for ; i < len(parts) && parts[i] != "على"; i++ {
		result.Counterparty += " " + parts[i]
	}

	i = 14
	result.AccountID = parts[i]
	i += 2

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i += 2

	timestamp, err := time.Parse("02-01-2006", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}

	return result, nil
}
