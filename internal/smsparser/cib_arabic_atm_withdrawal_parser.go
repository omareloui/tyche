package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBArabicATMWithdrawalParser struct{}

func (p *CIBArabicATMWithdrawalParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "تم سحب مبلغ") &&
		strings.Contains(sms, "من بطاقة الخصم المباشر المنتهية بـ")
}

func (p *CIBArabicATMWithdrawalParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.TransferTxKind, ToAccountID: "Cash"}

	parts := strings.Fields(sms)

	// تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A  ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00
	// 0  1   2     3   4       5  6     7     8       9        10 11     12 13  14     15 16  17 18       19    20 21    22     23  24

	i := 3

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	result.ToCurrency = result.Currency
	result.ToAmount = result.Amount

	i = 11
	result.AccountID = parts[i]
	i += 2

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

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
