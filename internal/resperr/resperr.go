package resperr

import "net/http"

type RespError struct {
	Code          int
	StatusMessage string
	Message       string
}

func (e RespError) Error() string {
	return "Http Error"
}

func New(code int, m string) *RespError {
	if m == "" {
		m = http.StatusText(code)
	}
	return &RespError{Code: code, StatusMessage: http.StatusText(code), Message: m}
}
