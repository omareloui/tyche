package valid

import (
	"reflect"
	"regexp"
	"slices"
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
		return v.parseError(input, err)
	}
	return nil
}

func (v *Validator) parseError(input any, error any) *errs.ValidationError {
	valerr := errs.ValidationErrors{}
	for _, err := range error.(validator.ValidationErrors) {
		valerr[fieldPath(reflect.TypeOf(input), err.StructNamespace())] = errs.ValidationField{
			Tag:   err.ActualTag(),
			Param: err.Param(),
		}
	}
	return &errs.ValidationError{Errors: valerr}
}

// fieldPath turns a field's namespace into its error key: it drops the root
// struct and embedded structs, so "createTransaction.commonFieldsForTxAndRtx.Amount"
// becomes "Amount" while "goal.Currency.ID" stays "Currency.ID".
func fieldPath(t reflect.Type, namespace string) string {
	segs := strings.Split(namespace, ".")[1:]
	path := make([]string, 0, len(segs))
	for _, seg := range segs {
		for t != nil && slices.Contains([]reflect.Kind{reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map}, t.Kind()) {
			t = t.Elem()
		}
		var f reflect.StructField
		found := false
		if t != nil && t.Kind() == reflect.Struct {
			name, _, _ := strings.Cut(seg, "[")
			f, found = t.FieldByName(name)
		}
		if found && f.Anonymous {
			t = f.Type
			continue
		}
		path = append(path, seg)
		if found {
			t = f.Type
		} else {
			t = nil
		}
	}
	return strings.Join(path, ".")
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
