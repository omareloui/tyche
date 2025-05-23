package models

import (
	"fmt"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type FrequencyBy uint8

const (
	FreqByNil FrequencyBy = iota
	FreqByDay
	FreqbyMonth
	FreqByYear
)

func (fb FrequencyBy) String() string {
	return [...]string{"???", "Day", "Month", "Year"}[fb]
}

func FrequencyByOptions() map[string]string {
	m := make(map[string]string, 3)
	m[fmt.Sprintf("%d", FreqByDay)] = FreqByDay.String()
	m[fmt.Sprintf("%d", FreqbyMonth)] = FreqbyMonth.String()
	m[fmt.Sprintf("%d", FreqByYear)] = FreqByYear.String()
	return m
}

type FrequentTx struct {
	ID          string
	FrequencyBy FrequencyBy
	Frequency   uint

	Account      Account
	Amount       float64
	Kind         TransactionKind
	Category     *Category
	ToAccount    *Account
	Counterparty *Counterparty
	ToAmount     float64
	Description  string
	Goal         *Goal

	StartsAt                   time.Time
	StopsAt                    time.Time
	CreatedAt                  time.Time
	LastTransactionScheduledTo time.Time

	Transactions []Transaction
}

type createFrequentTx struct {
	commonFieldsForTxAndFtx
	FrequencyBy FrequencyBy `validate:"required"`
	Frequency   uint        `validate:"required"`

	StartsAt                   time.Time `validate:"required"`
	StopsAt                    time.Time `validate:"omitempty,gtfield=StartsOn"`
	LastTransactionScheduledTo time.Time `validate:"required"`
}

func (ft *FrequentTx) ViewFrequency() string {
	return fmt.Sprintf("Each %d %s", ft.Frequency, ft.FrequencyBy)
}

func (ft *FrequentTx) ViewAmount() string {
	return formatAmount(ft.Amount, ft.Account.Currency.Symbol)
}

func (ft *FrequentTx) ViewToAmount() string {
	if ft.ToAccount == nil {
		return ""
	}
	return formatAmount(ft.ToAmount, ft.ToAccount.Currency.Symbol)
}

func (ft *FrequentTx) toValidate() *createFrequentTx {
	var catId, toAccId, cpId, glId string

	if cat := ft.Category; cat != nil {
		catId = cat.ID
	}
	if cp := ft.Counterparty; cp != nil {
		cpId = cp.ID
	}
	if toAcc := ft.ToAccount; toAcc != nil {
		toAccId = toAcc.ID
	}
	if gl := ft.Goal; gl != nil {
		glId = gl.ID
	}

	return &createFrequentTx{
		commonFieldsForTxAndFtx: commonFieldsForTxAndFtx{
			ID:             ft.ID,
			AccountID:      ft.Account.ID,
			Amount:         ft.Amount,
			KindID:         ft.Kind.ID,
			Kind:           ft.Kind.Enum,
			CategoryID:     catId,
			CounterpartyID: cpId,
			ToAccountID:    toAccId,
			ToAmount:       ft.ToAmount,
			Description:    ft.Description,
			GoalID:         glId,
		},
		FrequencyBy:                ft.FrequencyBy,
		Frequency:                  ft.Frequency,
		StartsAt:                   ft.StartsAt,
		StopsAt:                    ft.StopsAt,
		LastTransactionScheduledTo: ft.LastTransactionScheduledTo,
	}
}

func (ft *FrequentTx) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	ftc := ft.toValidate()
	if err := sani.SanitizeStruct(ftc); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(ftc); valerr != nil {
		return *valerr
	}
	return nil
}
