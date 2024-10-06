package repository

import "github.com/omareloui/tyche/internal/errs"

func newDuplicationError(flds ...string) *errs.ValidationError {
	valerr := &errs.ValidationError{}

	valerr.Errors = errs.ValidationErrors{}
	for _, f := range flds {
		valerr.Errors[f] = errs.ValidationField{Tag: "duplicate"}
	}

	return valerr
}
