package resperr

type RespError struct {
	Code int
}

func (e RespError) Error() string {
	return "Http Error"
}

func NewRespError(code int) *RespError {
	return &RespError{Code: code}
}
