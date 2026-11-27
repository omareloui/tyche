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
// (?t=&sort=&desc=1&col=&val=&p=), running their SQL (?sql=) or asking a
// question the AI turns into SQL (?ask=, see consoleAsk), read only.
// The console's own requests (HX-Target: console) get its body alone.
func (h *Handler) Console(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	q := r.URL.Query()

	tables, err := h.repo.ConsoleTables(ctx)
	if err != nil {
		return nil, err
	}
	data := views.ConsoleData{Tables: tables, AskOn: h.sqlGuess != nil}

	if q.Has("ask") {
		if err := h.consoleAsk(r, &data); err != nil {
			return nil, err
		}
	} else if q.Has("sql") {
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

// ConsoleDiagram is the SQL console's diagram of the user's tables and their
// foreign keys.
func (h *Handler) ConsoleDiagram(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	tables, err := h.repo.ConsoleSchema(ctx)
	if err != nil {
		return nil, err
	}
	data := views.ConsoleDiagramData{Tables: tables, Ranks: models.ConsoleRanks(tables), AskOn: h.sqlGuess != nil}
	if r.Header.Get("HX-Target") == views.ConsoleTarget {
		return views.ConsoleDiagramBody(data), nil
	}
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return views.ConsoleDiagram(getClaims(ctx), b, data), nil
}
