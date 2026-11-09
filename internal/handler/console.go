package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// Console is the SQL console: the user's database as is, browsing a table
// (?t=&sort=&desc=1&col=&val=&p=) or running their SQL (?sql=), read only
// (writes are ConsoleExec's).
// The console's own requests (HX-Target: console) get its body alone.
func (h *Handler) Console(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	q := r.URL.Query()

	tables, err := h.repo.ConsoleTables(ctx)
	if err != nil {
		return nil, err
	}
	data := views.ConsoleData{Tables: tables, CanUndo: h.repo.HasConsoleSnapshot(ctx)}

	if q.Has("sql") {
		data.Query = true
		data.SQL = q.Get("sql")
		if len(data.SQL) > models.ConsoleMaxSQL {
			return nil, resperr.New(http.StatusRequestEntityTooLarge, "The query is too long.")
		}
		if strings.TrimSpace(data.SQL) != "" {
			res, err := h.repo.ConsoleRun(ctx, data.SQL)
			if err != nil {
				if errors.Is(err, repository.ErrTenantLocked) || errors.Is(err, repository.ErrTenantDeleted) {
					return nil, err
				}
				data.Err = err.Error()
			} else {
				data.Result = &res
			}
		}
	} else if err := h.consoleBrowse(r, q, &data); err != nil {
		return nil, err
	}

	if r.Header.Get("HX-Target") == views.ConsoleTarget {
		return views.ConsoleBody(data), nil
	}
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return views.Console(getClaims(ctx), b, data), nil
}

// consoleBrowse loads into data the table q browses (?t=&sort=&desc=1&col=
// &val=&p=), the first one when it names none; the editor gets its query
// unless data has SQL already.
func (h *Handler) consoleBrowse(r *http.Request, q url.Values, data *views.ConsoleData) error {
	ctx := r.Context()
	name := q.Get("t")
	if name == "" {
		for _, t := range data.Tables {
			if !t.Internal {
				name = t.Name
				break
			}
		}
	}
	info, err := h.repo.ConsoleTableInfo(ctx, data.Tables, name)
	if errors.Is(err, repository.ErrConsoleNoTable) {
		return resperr.New(http.StatusNotFound, "There's no such table.")
	}
	if err != nil {
		return err
	}
	data.Table = &info

	data.Browse = models.ConsoleBrowse{
		Sort: q.Get("sort"),
		Desc: q.Get("desc") == "1",
		Col:  q.Get("col"),
		Val:  q.Get("val"),
	}
	data.Browse.Page, _ = strconv.Atoi(q.Get("p"))
	data.Browse.Page = max(data.Browse.Page, 0)
	if data.SQL == "" {
		data.SQL = data.Browse.Inline(info.Name)
	}

	res, total, err := h.repo.ConsoleBrowseTable(ctx, info, data.Browse)
	switch {
	case errors.Is(err, repository.ErrConsoleNoTable):
		return resperr.New(http.StatusNotFound, "There's no such column.")
	case errors.Is(err, repository.ErrTenantLocked), errors.Is(err, repository.ErrTenantDeleted):
		return err
	case err != nil:
		data.Err = err.Error()
	default:
		data.Result, data.Total = &res, total
	}
	return nil
}

// ConsoleExec runs the user's SQL in the console's write mode (confirm =
// models.ConsoleWriteConfirmation): its rows written in one transaction,
// undone by ConsoleUndo. Answers the console's body, back in read only.
func (h *Handler) ConsoleExec(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if r.PostFormValue("confirm") != models.ConsoleWriteConfirmation {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type "+models.ConsoleWriteConfirmation+" to turn the write mode on.")
	}
	data := views.ConsoleData{SQL: r.PostFormValue("sql")}
	if len(data.SQL) > models.ConsoleMaxSQL {
		return nil, resperr.New(http.StatusRequestEntityTooLarge, "The query is too long.")
	}

	var written *models.ConsoleResult
	var writeErr string
	if strings.TrimSpace(data.SQL) != "" {
		res, err := h.repo.ConsoleWrite(ctx, data.SQL)
		switch {
		case errors.Is(err, repository.ErrTenantLocked), errors.Is(err, repository.ErrTenantDeleted):
			return nil, err
		case err != nil:
			writeErr = err.Error()
		default:
			written = &res
			// The user row carries settings the claims have.
			if res.Changed > 0 {
				if err := h.refreshClaims(w, ctx); err != nil {
					return nil, err
				}
			}
		}
	}

	tables, err := h.repo.ConsoleTables(ctx)
	if err != nil {
		return nil, err
	}
	data.Tables, data.CanUndo = tables, h.repo.HasConsoleSnapshot(ctx)

	// A table browsed when it ran is loaded again, the write's outcome above
	// it, unless the SQL read rows (a SELECT, RETURNING): they're shown alone.
	if q, ok := consoleBrowsed(r); ok && (written == nil || len(written.Columns) == 0) {
		if err := h.consoleBrowse(r, q, &data); err != nil {
			var re *resperr.RespError
			if !errors.As(err, &re) {
				return nil, err
			}
			// The table went (e.g. not found): the write's outcome alone.
			data.Table, data.Result, data.Err = nil, nil, ""
		}
		if data.Table != nil {
			data.Written, data.WriteErr = written, writeErr
			return views.ConsoleBody(data), nil
		}
	}
	data.Query, data.Result, data.Err = true, written, writeErr
	// The url follows the SQL shown (reloading it runs it read only).
	w.Header().Set("HX-Push-Url", "/console?"+url.Values{"sql": {data.SQL}}.Encode())
	return views.ConsoleBody(data), nil
}

// consoleBrowsed is the query of the console page the request came from when
// it browses a table (not running SQL).
func consoleBrowsed(r *http.Request) (url.Values, bool) {
	u, err := url.Parse(r.Header.Get("HX-Current-URL"))
	if err != nil || u.Path != "/console" {
		return nil, false
	}
	q := u.Query()
	return q, !q.Has("sql")
}

// ConsoleUndo puts the database back as it was before the last console
// write; undoing again redoes it.
func (h *Handler) ConsoleUndo(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if !h.repo.HasConsoleSnapshot(r.Context()) {
		return nil, resperr.New(http.StatusNotFound, "There's no console write to undo.")
	}
	if err := h.repo.RestoreConsoleSnapshot(r.Context()); err != nil {
		return nil, err
	}
	// The access token carries settings of the user row.
	unsetCookie(w, accessTokenCookieName)
	w.Header().Set("HX-Redirect", "/console")
	return nil, nil
}

// ConsoleDiagram is the SQL console's diagram of the user's tables and their
// foreign keys.
func (h *Handler) ConsoleDiagram(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	tables, err := h.repo.ConsoleSchema(ctx)
	if err != nil {
		return nil, err
	}
	data := views.ConsoleDiagramData{Tables: tables, Ranks: models.ConsoleRanks(tables)}
	if r.Header.Get("HX-Target") == views.ConsoleTarget {
		return views.ConsoleDiagramBody(data), nil
	}
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return views.ConsoleDiagram(getClaims(ctx), b, data), nil
}
