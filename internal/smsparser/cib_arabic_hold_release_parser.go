package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicHoldReleaseParser struct{}

func (p *CIBArabicHoldReleaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "تم اضافة المعاملة من") &&
		strings.Contains(sms, "إلى بطاقتك المنتهية بـ") &&
		strings.Contains(sms, "الرصيد المتاح")
}

func (p *CIBArabicHoldReleaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.IncomeTxKind}

	parts := strings.Fields(sms)

	// تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10
	// 0  1     2        3  4    5       6 7     8     9   10  11     12       13 14     15 16    17     18  19

	i := 4

	result.Counterparty = parts[i]
	i++
	for ; i < len(parts) && parts[i] != "بقيمة"; i++ {
		result.Counterparty += " " + parts[i]
	}
	i++

	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i++
	result.Currency = parts[i]
	i += 5

	result.AccountID = parts[i][2:]

	i += 4

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	// TODO: get the sms timestamp instead of using time.Now()
	result.Timestamp = time.Now()

	return result, nil
}
