package handler

import (
	"net/http"

	"github.com/a-h/templ"
)

type (
	HandlerMethod     func(w http.ResponseWriter, r *http.Request) (templ.Component, error)
	HandlerJSONMethod func(w http.ResponseWriter, r *http.Request) ([]byte, error)
)
