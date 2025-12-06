package smsparser

import (
	"errors"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishDebitCardPurchaseParser struct{}

func (p *HSBCEnglishDebitCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "From HSBC:") &&
		strings.Contains(sms, "Purchase from")
}

func (p *HSBCEnglishDebitCardPurchaseParser) Parse(sms string) (*SMS, error) {
	result := &SMS{Bank: HSBC, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)
	partsLen := len(parts)

	if partsLen < 4 {
		return nil, errors.New("invalid HSBC English Debit Card Purchase message")
	}

	i := 2

	timestamp, err := time.Parse("02Jan06", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}
	i++

	result.Counterparty = parts[i]
	i++
	for ; i < partsLen && parts[i] != "Purchase"; i++ {
		result.Counterparty += " " + parts[i]
	}
	i += 2

	result.AccountID = parts[i]
	i++

	result.Currency = parts[i]
	i++
	result.Amount = amountStrToFloat(parts[i])
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance = amountStrToFloat(parts[i])

	return result, nil
}
