package errs

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidID     = errors.New("invalid id")
	ErrInvalidFloat  = errors.New("invalid float")
	ErrInvalidNumber = errors.New("invalid number")
	ErrInvalidDate   = errors.New("invalid date")
)

type ValidationErrors map[string]ValidationField

type ValidationField struct {
	Tag   string
	Param string
}

func (e *ValidationErrors) MsgFor(fieldName string) string {
	f, ok := (*e)[fieldName]
	if !ok {
		return ""
	}
	return f.String()
}

func (v ValidationField) String() string {
	if v.Tag == "" {
		return ""
	}

	switch v.Tag {
	case "required", "required_if":
		return "This field is required"
	case "excluded_unless":
		return fmt.Sprintf("This field can't be included on %s", v.Param)
	case "email":
		return "Invalid email"
	case "ulid":
		return "Invalid ID"
	case "http_url":
		return "This field must be a valid URL"
	case "gte":
		return fmt.Sprintf("Value is low (at least %s is required)", v.Param)
	case "min":
		var pluralS string
		if v.Param != "1" {
			pluralS = "s"
		}
		return fmt.Sprintf("Value is too short (at least %s element%s)", v.Param, pluralS)
	case "max":
		return fmt.Sprintf("Value is too log (maximum %s elements)", v.Param)
	case "eqfield":
		return fmt.Sprintf(`This field must match the "%s" field`, v.Tag)
	case "not_blank":
		return "This field can't be empty"
	case "alphanum":
		return "This field expects alphanumeric value"
	case "alphanum_with_underscore":
		return "This field expects alphanumeric or underscore characters"
	case "mongodb":
		return "Invalid ID"
	case "oneof":
		return fmt.Sprintf(`This field must be one of %s`, v.Param)
	case "gtcsfield", "gtfield":
		return fmt.Sprintf(`This field has to be greater than %s`, v.Param)
	case "duplicate":
		return "This field is in use, try another one"
	case "credit_card":
		return "This field has to be a valid card number"
	case "brand_icon":
		return "Unknown icon, use a slug from simpleicons.org or leave it empty"
	default:
		msg := fmt.Sprintf(`Failed on "%s" tag`, v.Tag)
		if v.Param != "" {
			msg += fmt.Sprintf(` with "%s" as a param`, v.Param)
		}
		return msg
	}
}

type ValidationError struct {
	Errors ValidationErrors
}

func (v ValidationError) Error() string {
	return "Validation Error"
}
