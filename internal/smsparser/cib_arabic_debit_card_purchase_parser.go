package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicDebitCardPurchaseParser struct{}

func (p *CIBArabicDebitCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "تم خصم مبلغ") &&
		strings.Contains(sms, "من بطاقة الخصم المباشر المنتهية بـ")
}

func (p *CIBArabicDebitCardPurchaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)

	// تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP
	// 0  1   2     3   4      5  6     7     8       9        10 11     12  13      14    15 16       17    18 19    20     21      22

	i := 3

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	i = 11
	result.AccountID = parts[i][2:]

	i = 13
	result.Counterparty = parts[i]
	i++

	for ; i < len(parts) && parts[i] != "في"; i++ {
		result.Counterparty += " " + parts[i]
	}
	i++

	timestamp, err := time.Parse("02/01/06 15:04", parts[i]+" "+parts[i+1])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 5

	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i++
	result.AvailableBalanceCurrency = parts[i]

	return result, nil
}
