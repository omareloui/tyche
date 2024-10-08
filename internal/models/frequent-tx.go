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

	StartsAt                   time.Time
	StopsAt                    time.Time
	CreatedAt                  time.Time
	LastTransactionScheduledTo time.Time

	Transactions []Transaction
}

type createFrequentTx struct {
	ID          string      `validate:"required,ulid"`
	FrequencyBy FrequencyBy `validate:"required"`
	Frequency   uint        `validate:"required"`

	AccountID      string  `validate:"required,ulid"`
	Amount         float64 `validate:"required"`
	KindID         string  `validate:"required,ulid"`
	Kind           TxKind  `validate:"required"`
	CategoryID     string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	CounterpartyID string  `validate:"excluded_if=Kind 3,excluded_if=Kind 4,omitempty,ulid"`
	ToAccountID    string  `validate:"required_if=Kind 3,required_if=Kind 4,omitempty,ulid"`
	ToAmount       float64 `validate:"required_if=Kind 3,required_if=Kind 4"`
	Description    string  `conform:"trim"`

	StartsAt                   time.Time `validate:"required"`
	StopsAt                    time.Time `validate:"omitempty,gtfield=StartsOn"`
	LastTransactionScheduledTo time.Time `validate:"required"`
}

func (ft *FrequentTx) ViewFrequency() string {
	return fmt.Sprintf("Each %d %s", ft.Frequency, ft.FrequencyBy)
}

func (ft *FrequentTx) ViewAmount() string {
	return formatAmount(ft.Amount, ft.Account.Currency.Enum)
}

func (ft *FrequentTx) ViewToAmount() string {
	if ft.ToAccount == nil {
		return ""
	}
	return formatAmount(ft.ToAmount, ft.ToAccount.Currency.Enum)
}

func (ft *FrequentTx) toValidate() *createFrequentTx {
	var catId, toAccId, cpId string

	if cat := ft.Category; cat != nil {
		catId = cat.ID
	}
	if cp := ft.Counterparty; cp != nil {
		cpId = cp.ID
	}
	if toAcc := ft.ToAccount; toAcc != nil {
		toAccId = toAcc.ID
	}

	return &createFrequentTx{
		ID:                         ft.ID,
		AccountID:                  ft.Account.ID,
		Amount:                     ft.Amount,
		KindID:                     ft.Kind.ID,
		Kind:                       ft.Kind.Enum,
		CategoryID:                 catId,
		CounterpartyID:             cpId,
		ToAccountID:                toAccId,
		ToAmount:                   ft.ToAmount,
		Description:                ft.Description,
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
