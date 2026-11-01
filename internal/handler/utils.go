package handler

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
)

var nonDigitRegexp *regexp.Regexp = regexp.MustCompile(`[^0-9\.]`)

func getClaims(ctx context.Context) *jwt.AccessClaims {
	return middleware.ClaimsFromCtx(ctx)
}

func renderToBody(w http.ResponseWriter, r *http.Request, template templ.Component) error {
	return template.Render(r.Context(), w)
}

func hxRespondWithRedirect(w http.ResponseWriter, path string) {
	w.Header().Set("HX-Location", path)
}

func hxRespondWithRefresh(w http.ResponseWriter) {
	w.Header().Set("HX-Refresh", "true")
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

func parseUintIfExists(str string) (uint64, error) {
	str = nonDigitRegexp.ReplaceAllLiteralString(str, "")
	if str != "" {
		num, err := strconv.ParseUint(str, 10, 64)
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

// parseSignedFloatIfExists is parseFloatIfExists keeping a leading minus.
func parseSignedFloatIfExists(str string) (float64, error) {
	str = strings.TrimSpace(str)
	num, err := parseFloatIfExists(str)
	if strings.HasPrefix(str, "-") || strings.HasPrefix(str, "−") {
		num = -num
	}
	return num, err
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

// keepTimeOfDay returns date's day with timeOf's time of day, in timeOf's
// location. date is expected to be a date-only value (midnight UTC).
func keepTimeOfDay(date, timeOf time.Time) time.Time {
	if date.IsZero() || timeOf.IsZero() {
		return date
	}
	y, m, d := date.Date()
	return time.Date(y, m, d, timeOf.Hour(), timeOf.Minute(), timeOf.Second(), timeOf.Nanosecond(), timeOf.Location())
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
