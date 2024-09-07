package response_handler

import (
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/financial-plans/internal/resperr"
)

type ResponseHandler struct{}

func NewResponseHandler() *ResponseHandler { return new(ResponseHandler) }

func (rh *ResponseHandler) Handler(fun func(w http.ResponseWriter, r *http.Request) (templ.Component, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s\n", r.URL)
		comp, err := fun(w, r)
		if err != nil {
			var httperr *resperr.RespError
			if errors.As(err, &httperr) {
				rh.ErrorResponse(w, r, httperr.Code)
				return
			}
			rh.ErrorResponse(w, r, http.StatusInternalServerError)
			return
		}

		if err := comp.Render(r.Context(), w); err != nil {
			rh.ErrorResponse(w, r, http.StatusInternalServerError)
			return
		}

		rh.PrepareResponse(w, 0)
	}
}

func (rh *ResponseHandler) ErrorResponse(w http.ResponseWriter, r *http.Request, code int) {
	rh.PrepareResponse(w, code)
	_, err := io.WriteString(w, http.StatusText(code))
	if err != nil {
		rh.ErrorResponse(w, r, http.StatusInternalServerError)
	}
}

func (rh *ResponseHandler) OkResponse(w http.ResponseWriter, r *http.Request, body string) {
	rh.PrepareResponse(w, http.StatusOK)
	_, err := io.WriteString(w, body)
	if err != nil {
		rh.ErrorResponse(w, r, http.StatusInternalServerError)
	}
}

func (rh *ResponseHandler) PrepareResponse(w http.ResponseWriter, code int) {
	if code != 0 {
		w.WriteHeader(code)
	}
	w.Header().Set("Content-Type", "text/html")
}
