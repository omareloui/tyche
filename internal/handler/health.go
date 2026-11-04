package handler

import (
	"encoding/json"
	"net/http"

	"github.com/omareloui/tyche/internal/buildinfo"
)

type HealthCheckResp struct {
	Status string `json:"status"`
	// Commit is the git commit being served, which the deploy waits for.
	Commit string `json:"commit"`
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	w.Header().Set("Cache-Control", "no-store")
	st := HealthCheckResp{
		Status: "ok",
		Commit: buildinfo.Commit,
	}
	return json.Marshal(st)
}
