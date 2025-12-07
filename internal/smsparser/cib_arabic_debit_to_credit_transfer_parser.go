package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicDebitToCreditTransferParser struct{}

func (p *CIBArabicDebitToCreditTransferParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "تم سداد مبلغ") &&
		strings.Contains(sms, "فى بطاقتكم الائتمانية المنتهية بـ")
}

func (p *CIBArabicDebitToCreditTransferParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	parts := strings.Fields(sms)

	// تم سداد مبلغ 500.00 جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ 05-12-25
	// 0  1    2    3      4  5  6       7         8        9  10   11     12

	result := &SMS{Bank: CIB, TxKind: models.TransferTxKind}

	i := 3

	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i++

	result.Currency = convertArabicCurrencyToEnglish(parts[i])

	result.ToAmount = result.Amount
	result.ToCurrency = result.Currency

	i = 10
	result.ToAccountID = parts[i]
	i += 2

	timestamp, err := time.Parse("02-01-06", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}

	return result, nil
}
