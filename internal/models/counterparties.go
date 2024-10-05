package models

import (
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Counterparty struct {
	ID        string `validate:"required"`
	Name      string `validate:"required,not_blank" conform:"trim,title"`
	CreatedAt time.Time
}

func (cp *Counterparty) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	if err := sani.SanitizeStruct(cp); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(cp); valerr != nil {
		return *valerr
	}
	return nil
}
