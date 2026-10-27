package smsparser

import (
	"regexp"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type AAIBEnglishCreditCardPurchaseParser struct{}

var aaibCCPurchaseRe = regexp.MustCompile(
	`^Your credit card \*\*(\d+) was debited by (\d[\d ,]*(?:\.\d+)?)([A-Z]{3}) at (.+?)\. Available limit is (\d[\d ,]*(?:\.\d+)?)`)

func (p *AAIBEnglishCreditCardPurchaseParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your credit card") &&
		strings.Contains(sms, "was debited by")
}

func (p *AAIBEnglishCreditCardPurchaseParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	// Amounts may use a space as the thousands separator:
	// Your credit card **1432 was debited by 2 111.00EGP at PAYMOBS*Sympl. Available limit is 1 284.40. To install it up to 60 months, call 19555. T&Cs apply.
	m := aaibCCPurchaseRe.FindStringSubmatch(sms)
	if m == nil {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: AAIB, TxKind: models.ExpenseTxKind, AccountID: m[1], Currency: m[3]}

	result.Amount, err = amountStrToFloat(m[2])
	if err != nil {
		return nil, err
	}

	result.Counterparty = strings.TrimSpace(m[4])

	result.AvailableBalance, err = amountStrToFloat(m[5])
	if err != nil {
		return nil, err
	}

	// TODO: get the sms timestamp instead of using time.Now()
	result.Timestamp = time.Now()

	return result, nil
}
