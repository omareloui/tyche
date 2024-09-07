package response_handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/resperr"
)

type ResponseHandler struct{}

func NewResponseHandler() *ResponseHandler {
	return &ResponseHandler{}
}

func (rh *ResponseHandler) Handler(fun func(w http.ResponseWriter, r *http.Request) (templ.Component, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s\n", r.URL)
		comp, err := fun(w, r)
		if err != nil {
			var httperr *resperr.RespError
			if errors.As(err, &httperr) {
				rh.ErrorResponse(w, httperr)
				return
			}
			rh.ErrorResponse(w, resperr.NewRespError(http.StatusInternalServerError))
			return
		}

		if err := comp.Render(r.Context(), w); err != nil {
			rh.ErrorResponse(w, resperr.NewRespError(http.StatusInternalServerError))
			return
		}

		rh.PrepareResponse(w, 0)
	}
}

func (rh *ResponseHandler) ErrorResponse(w http.ResponseWriter, err *resperr.RespError) {
	http.Error(w, http.StatusText(err.Code), err.Code)
}

func (rh *ResponseHandler) PrepareResponse(w http.ResponseWriter, code int) {
	if code != 0 {
		w.WriteHeader(code)
	}
	w.Header().Add("Content-Type", "text/html")
}
