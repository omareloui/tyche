package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type AAIBEnglishCreditCardPurchaseParser struct{}

func (p *AAIBEnglishCreditCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your credit card") &&
		strings.Contains(sms, "was debited by")
}

func (p *AAIBEnglishCreditCardPurchaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: AAIB, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)

	// Your credit card **1432 was debited by 229.00EGP at Vodafone Top Up APP. Available limit is 32.70.
	// 0    1      2    3      4   5       6  7         8  9        10  11 12   13        14    15 16

	i := 3

	result.AccountID = parts[i][2:]

	i = 7

	result.Currency = parts[i][len(parts[i])-3:]
	result.Amount, err = amountStrToFloat(parts[i][:len(parts[i])-3])
	if err != nil {
		return nil, err
	}

	i = 9

	result.Counterparty = parts[i]
	if !strings.HasSuffix(result.Counterparty, ".") {
		i++
		for ; i < len(parts) && !strings.HasSuffix(parts[i], "."); i++ {
			result.Counterparty += " " + parts[i]
		}
		result.Counterparty += " " + parts[i]
	}
	result.Counterparty = strings.TrimRight(result.Counterparty, ".")

	i += 4

	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	// TODO: get the sms timestamp instead of using time.Now()
	result.Timestamp = time.Now()

	return result, nil
}
