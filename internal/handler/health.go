package handler

import (
	"encoding/json"
	"net/http"
)

type HealthCheckResp struct {
	Status string `json:"status"`
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	st := HealthCheckResp{
		Status: "ok",
	}
	return json.Marshal(st)
}
