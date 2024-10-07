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
	Day FrequencyBy = iota + 1
	Month
	Year
)

func (fb FrequencyBy) String() string {
	return [...]string{"Day", "Month", "Year"}[fb-1]
}

func FrequencyByOptions() map[string]string {
	m := make(map[string]string, 3)
	m[fmt.Sprintf("%d", Day)] = Day.String()
	m[fmt.Sprintf("%d", Month)] = Month.String()
	m[fmt.Sprintf("%d", Year)] = Year.String()
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

	StartsOn                 time.Time
	StopsOn                  time.Time
	CreatedAt                time.Time
	LastTransactionCreatedAt time.Time

	Transactions []Transaction
}

type createFrequentTx struct {
	ID          string      `validate:"required,ulid"`
	FrequencyBy FrequencyBy `validate:"required,oneof=0 1 2"`
	Frequency   uint        `validate:"required"`

	AccountID      string  `validate:"required,ulid"`
	Amount         float64 `validate:"required"`
	KindID         string  `validate:"required,ulid"`
	KindString     string  `validate:"required,oneof=EXPENSE INCOME TRANSFER DEBT"`
	CategoryID     string  `validate:"excluded_if=KindString TRANSFER,omitempty,ulid"`
	CounterpartyID string  `validate:"excluded_if=KindString TRANSFER,omitempty,ulid"`
	ToAccountID    string  `validate:"required_if=KindString TRANSFER,omitempty,ulid"`
	ToAmount       float64 `validate:"required_if=KindString TRANSFER"`
	Description    string  `conform:"trim"`

	StartsOn                 time.Time
	StopsOn                  time.Time `validate:"omitempty,gtfield=StartsOn"`
	LastTransactionCreatedAt time.Time `validate:"required"`
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
		ID:                       ft.ID,
		AccountID:                ft.Account.ID,
		Amount:                   ft.Amount,
		KindID:                   ft.Kind.ID,
		KindString:               ft.Kind.Kind,
		CategoryID:               catId,
		CounterpartyID:           cpId,
		ToAccountID:              toAccId,
		ToAmount:                 ft.ToAmount,
		Description:              ft.Description,
		StartsOn:                 ft.StartsOn,
		FrequencyBy:              ft.FrequencyBy,
		Frequency:                ft.Frequency,
		StopsOn:                  ft.StopsOn,
		LastTransactionCreatedAt: ft.LastTransactionCreatedAt,
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
