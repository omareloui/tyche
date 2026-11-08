package handler

import (
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/logview"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/sysinfo"
	"github.com/omareloui/tyche/web/views"
)

func (h Handler) AdminLogs(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	d, err := adminLogsData(r)
	if err != nil {
		return nil, err
	}
	d.Sys = sysinfo.Read()
	return views.AdminLogs(getClaims(r.Context()), b, d), nil
}

// AdminLogsList answers the list for the filters, or the rows of an older
// page (before set).
func (h Handler) AdminLogsList(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := adminLogsData(r)
	if err != nil {
		return nil, err
	}
	if d.Filter.Before > 0 {
		return views.AdminLogRows(d), nil
	}
	w.Header().Set("HX-Replace-Url", "/admin/logs?"+d.Query(0))
	return views.AdminLogsList(d), nil
}

func adminLogsData(r *http.Request) (views.AdminLogsData, error) {
	q := r.URL.Query()
	files, err := logview.Files(logger.Dir)
	if err != nil {
		return views.AdminLogsData{}, err
	}
	d := views.AdminLogsData{
		Files: files,
		File:  q.Get("file"),
		Live:  q.Get("live") == "1",
		Filter: logview.Filter{
			Query:         q.Get("q"),
			CorrelationID: q.Get("cid"),
		},
	}
	if d.File == "" {
		d.File = logview.Live
	}
	if l := q.Get("level"); slices.Contains(logview.Levels, l) {
		d.Filter.Level = l
	}
	if s := q.Get("before"); s != "" {
		if d.Filter.Before, err = strconv.ParseInt(s, 10, 64); err != nil || d.Filter.Before < 0 {
			return d, resperr.New(http.StatusBadRequest, "invalid before")
		}
	}

	d.Page, err = logview.Read(logger.Dir, d.File, d.Filter)
	if errors.Is(err, logview.ErrNoFile) {
		if d.File == logview.Live {
			// Nothing logged to a file yet.
			return d, nil
		}
		return d, resperr.New(http.StatusNotFound, "no such log file")
	}
	return d, err
}
