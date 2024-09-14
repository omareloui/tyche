package handler

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/errs"
)

var nonDigitRegexp *regexp.Regexp = regexp.MustCompile(`[^0-9\.]`)

func renderToBody(w http.ResponseWriter, r *http.Request, template templ.Component) error {
	return template.Render(r.Context(), w)
}

func hxRespondWithRedirect(w http.ResponseWriter, path string) error {
	w.Header().Set("HX-Location", path)
	return nil
}

func parseIntIfExists(str string) (int, error) {
	str = nonDigitRegexp.ReplaceAllLiteralString(str, "")
	if str != "" {
		num, err := strconv.Atoi(str)
		if err != nil {
			return 0, errs.ErrInvalidNumber
		}
		return num, nil
	}
	return 0, nil
}

func parseBoolean(str string) bool {
	return str == "on"
}

func parseFloatIfExists(str string) (float64, error) {
	if str != "" {
		str = nonDigitRegexp.ReplaceAllLiteralString(str, "")
		num, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, errs.ErrInvalidFloat
		}
		return num, nil
	}
	return 0, nil
}

func parseDateOnlyIfExists(str string) (time.Time, error) {
	if str != "" {
		date, err := time.Parse(time.DateOnly, str)
		if err != nil {
			return time.Time{}, errs.ErrInvalidFloat
		}
		return date, nil
	}
	return time.Time{}, nil
}

func formatDateOnlyIfNonZero(date time.Time) string {
	if date.IsZero() {
		return ""
	}
	return date.Format(time.DateOnly)
}

func formatIntIfNonZero(i int) string {
	if i == 0 {
		return ""
	}
	return strconv.Itoa(i)
}

func formatFloatIfNonZero(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatBooleanIfNonZero(b bool) string {
	if !b {
		return ""
	}
	return "on"
}
