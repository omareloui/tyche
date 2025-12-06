package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishATMWithdrawalParser struct{}

func (p *HSBCEnglishATMWithdrawalParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "From HSBC:") &&
		strings.Contains(sms, "ATM Cash Withdrawal from") &&
		!strings.Contains(sms, "Reversal")
}

func (p *HSBCEnglishATMWithdrawalParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.TransferTxKind, ToAccountID: "Cash"}

	parts := strings.Fields(sms)

	// From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00- Your available balance is EGP 27,905.35
	// 0    1     2       3   4    5          6    7              8   9         10   11        12      13 14  15

	i := 2

	timestamp, err := time.Parse("02Jan06", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 5

	result.AccountID = parts[i]
	i++

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	result.ToCurrency = result.Currency
	result.ToAmount = result.Amount
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
