package valid

import (
	"regexp"
	"strings"

	"github.com/almerlucke/go-iban/iban"
	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/non-standard/validators"
	"github.com/omareloui/tyche/internal/errs"
)

type Validator struct {
	validator *validator.Validate
}

func (v *Validator) Validate(input any) *errs.ValidationError {
	if err := v.validator.Struct(input); err != nil {
		return v.parseError(err)
	}
	return nil
}

func (v *Validator) parseError(error any) *errs.ValidationError {
	valerr := errs.ValidationErrors{}
	for _, err := range error.(validator.ValidationErrors) {
		namespace := err.Namespace()
		firstDot := strings.Index(namespace, ".")
		path := namespace[firstDot+1:]
		valerr[path] = errs.ValidationField{
			Tag:   err.ActualTag(),
			Param: err.Param(),
		}
	}
	return &errs.ValidationError{Errors: valerr}
}

func New() *Validator {
	val := validator.New(validator.WithRequiredStructEnabled())
	_ = val.RegisterValidation("not_blank", validators.NotBlank)
	_ = val.RegisterValidation("alphanum_with_underscore", IsAlphaNumWithUnderScore)
	_ = val.RegisterValidation("iban", IsIban)
	return &Validator{validator: val}
}

func IsAlphaNumWithUnderScore(fl validator.FieldLevel) bool {
	re := regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	field := fl.Field()
	return re.Match([]byte(field.String()))
}

func IsIban(fl validator.FieldLevel) bool {
	field := fl.Field().String()
	_, err := iban.NewIBAN(field)
	return err != nil
}
