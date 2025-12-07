package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBEnglishCreditCardPurchaseParser struct{}

func (p *CIBEnglishCreditCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your credit card ending with#") &&
		strings.Contains(sms, "was charged for")
}

func (p *CIBEnglishCreditCardPurchaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: CIB, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)

	// Your credit card ending with#5678 was charged for EGP 150.00 at Generic Store ABC on 04/12/25  at 10:48. Card available limit is EGP  95000.00. For more details, please visit https://cib.eg/mb
	// 0    1      2    3      4         5   6       7   8   9      10 11      12    13  14 15        16 17     19   20        21    22 23  24         25  26   27       28     29    30

	i := 4

	result.AccountID = strings.TrimPrefix(parts[i], "with#")

	i = 8
	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	i = 11

	result.Counterparty = parts[i]
	i++
	for ; i < len(parts) && parts[i] != "on"; i++ {
		result.Counterparty += " " + parts[i]
	}
	i++

	timestamp, err := time.Parse("02/01/06 15:04", parts[i]+" "+strings.TrimRight(parts[i+2], "."))
	if err == nil {
		result.Timestamp = timestamp
	}

	i += 7

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
