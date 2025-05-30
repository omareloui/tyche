package models

import (
	"fmt"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type RecurrentUnit uint8

const (
	RecurrentUnitNil RecurrentUnit = iota
	RecurrentUnitDay
	RecurrentUnitMonth
	RecurrentUnitYear
)

func (fb RecurrentUnit) String() string {
	return [...]string{"???", "Day", "Month", "Year"}[fb]
}

func RecurrentUnitOptions() map[string]string {
	m := make(map[string]string, 3)
	m[fmt.Sprintf("%d", RecurrentUnitDay)] = RecurrentUnitDay.String()
	m[fmt.Sprintf("%d", RecurrentUnitMonth)] = RecurrentUnitMonth.String()
	m[fmt.Sprintf("%d", RecurrentUnitYear)] = RecurrentUnitYear.String()
	return m
}

type RecurrentTx struct {
	ID    string
	Unit  RecurrentUnit
	Every uint

	Account      Account
	Amount       float64
	Kind         TransactionKind
	Description  string
	ToAmount     float64
	ToAccount    *Account
	Category     *Category
	Counterparty *Counterparty
	Goal         *Goal

	StartsAt                   time.Time
	StopsAt                    time.Time
	CreatedAt                  time.Time
	LastTransactionScheduledTo time.Time
}

type createRecurrentTx struct {
	commonFieldsForTxAndRtx
	Unit  RecurrentUnit `validate:"required"`
	Every uint          `validate:"required"`

	StartsAt                   time.Time `validate:"required"`
	StopsAt                    time.Time `validate:"omitempty,gtfield=StartsOn"`
	LastTransactionScheduledTo time.Time `validate:"required"`
}

func (rt *RecurrentTx) ViewRecurrence() string {
	extraS := ""
	if rt.Every != 1 {
		extraS = "s"
	}
	return fmt.Sprintf("Every %d %s%s", rt.Every, rt.Unit, extraS)
}

func (rt *RecurrentTx) ViewAmount() string {
	return formatAmount(rt.Amount, rt.Account.Currency.Symbol)
}

func (rt *RecurrentTx) ViewToAmount() string {
	if rt.ToAccount == nil {
		return ""
	}
	return formatAmount(rt.ToAmount, rt.ToAccount.Currency.Symbol)
}

func (rt *RecurrentTx) toValidate() *createRecurrentTx {
	var catId, toAccId, cpId, glId string

	if cat := rt.Category; cat != nil {
		catId = cat.ID
	}
	if cp := rt.Counterparty; cp != nil {
		cpId = cp.ID
	}
	if toAcc := rt.ToAccount; toAcc != nil {
		toAccId = toAcc.ID
	}
	if gl := rt.Goal; gl != nil {
		glId = gl.ID
	}

	return &createRecurrentTx{
		commonFieldsForTxAndRtx: commonFieldsForTxAndRtx{
			ID:             rt.ID,
			AccountID:      rt.Account.ID,
			Amount:         rt.Amount,
			KindID:         rt.Kind.ID,
			CategoryID:     catId,
			CounterpartyID: cpId,
			ToAccountID:    toAccId,
			ToAmount:       rt.ToAmount,
			Description:    rt.Description,
			GoalID:         glId,
		},
		Unit:                       rt.Unit,
		Every:                      rt.Every,
		StartsAt:                   rt.StartsAt,
		StopsAt:                    rt.StopsAt,
		LastTransactionScheduledTo: rt.LastTransactionScheduledTo,
	}
}

func (rt *RecurrentTx) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	cRt := rt.toValidate()
	if err := sani.SanitizeStruct(cRt); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(cRt); valerr != nil {
		return *valerr
	}
	return nil
}
