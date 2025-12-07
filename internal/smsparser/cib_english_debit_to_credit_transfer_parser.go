package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type CIBEnglishDebitToCreditTransferParser struct{}

func (p *CIBEnglishDebitToCreditTransferParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your account ending with") &&
		strings.Contains(sms, "is debited with amount") &&
		strings.Contains(sms, "with transfer to  another account")
}

func (p *CIBEnglishDebitToCreditTransferParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	parts := strings.Fields(sms)

	// Your account ending with ********1234 is debited with amount EGP 750.00DR On 07 DEC 2025 with transfer to  another account.  For more information, please contact 19666.
	// 0    1       2      3    4            5  6       7    8      9   10       11 12 13  14   15   16       17  18      19        20  21   22           23     24      25

	result := &SMS{Bank: CIB, TxKind: models.TransferTxKind}

	i := 4

	result.AccountID = parts[i][8:]

	i = 9

	result.Currency = parts[i]
	i++
	amtStr := strings.TrimSuffix(parts[i], "DR")
	result.Amount, err = amountStrToFloat(amtStr)
	if err != nil {
		return nil, err
	}

	result.ToAmount = result.Amount
	result.ToCurrency = result.Currency

	i = 12
	timestamp, err := time.Parse("02 Jan 2006", parts[i]+" "+parts[i+1]+" "+parts[i+2])
	if err == nil {
		result.Timestamp = timestamp
	}

	return result, nil
}
