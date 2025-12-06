package smsparser

import (
	"errors"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishCreditCardPurchaseParser struct{}

func (p *HSBCEnglishCreditCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your Credit Card ending with") &&
		strings.Contains(sms, "has been used for")
}

func (p *HSBCEnglishCreditCardPurchaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)
	partsLen := len(parts)

	if partsLen < 6 {
		return nil, errors.New("invalid HSBC English Credit Card Purchase message")
	}

	i := 6

	result.AccountID = parts[i]
	i += 5

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i += 2

	timestamp, err := time.Parse("02/01/2006", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 2

	result.Counterparty = parts[i]
	if !strings.HasSuffix(result.Counterparty, ".") {
		i++
		for ; i < partsLen && !strings.HasSuffix(parts[i], "."); i++ {
			result.Counterparty += " " + parts[i]
		}
		result.Counterparty += " " + parts[i]
	}
	result.Counterparty = strings.TrimRight(result.Counterparty, ".")
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
