package repository

import "github.com/omareloui/financial-planner/internal/errs"

func newDuplicationError(f string) *errs.ValidationError {
	valerr := &errs.ValidationError{}
	valerr.Errors = errs.ValidationErrors{
		f: errs.ValidationField{Tag: "duplicate"},
	}
	return valerr
}
