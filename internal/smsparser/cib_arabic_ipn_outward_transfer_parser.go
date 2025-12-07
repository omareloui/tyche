package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicIPNOutwardTransferParser struct{}

func (p *CIBArabicIPNOutwardTransferParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ") &&
		strings.Contains(sms, "من حسابك المنتهي بـ")
}

func (p *CIBArabicIPNOutwardTransferParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)

	// يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666
	// 0    1     2   3  4     5     6    7     8      9  10 11    12      13 14           15   16    17       18     19         20    21       22   23     24 25

	i := 8

	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i++

	result.Currency = convertArabicCurrencyToEnglish(parts[i])

	i = 14
	result.AccountID = parts[i][8:]
	i++

	i += 2
	result.Ref = parts[i]
	i += 2

	timestamp, err := time.Parse("02-01-2006 15:04", parts[i]+" "+parts[i+1])
	if err == nil {
		result.Timestamp = timestamp
	}

	return result, nil
}
