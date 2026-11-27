package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/sqlguess"
	"github.com/omareloui/tyche/web/views"
)

// askTimeout is how long writing a question's SQL may take, each try.
const askTimeout = 30 * time.Second

// SetSQLGuess turns the console's Ask tab on (a nil guesser keeps it off).
func (h *Handler) SetSQLGuess(g sqlguess.Guesser) {
	h.sqlGuess = g
}

// consoleAsk is the console's Ask tab (?ask=): the question turned into SQL
// by the AI and run in the console's sandbox, one retry told the error when
// it fails; a question asked before reruns its SQL from the history without
// a call. Only the question and the schema are sent, never a row.
func (h *Handler) consoleAsk(r *http.Request, data *views.ConsoleData) error {
	ctx := r.Context()
	if h.sqlGuess == nil {
		return resperr.New(http.StatusNotFound, "Asking is off.")
	}
	ask := &views.ConsoleAsk{Question: sqlguess.CleanQuestion(r.URL.Query().Get("ask"))}
	data.Ask = ask

	if err := h.askAnswer(ctx, data); err != nil {
		return err
	}

	var err error
	if ask.History, err = h.repo.GetConsoleQuestions(ctx); err != nil {
		return err
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanAsk, h.ocrCfg.AskQuestionsPerMonth, time.Now())
	if err != nil {
		return err
	}
	ask.Used, ask.Limit = limit-left, limit
	return nil
}

// askAnswer answers data.Ask's question into data (its SQL, rows or error).
func (h *Handler) askAnswer(ctx context.Context, data *views.ConsoleData) error {
	ask := data.Ask
	l := logger.FromCtx(ctx)
	if ask.Question == "" {
		return nil
	}
	if sqlguess.TooLong(ask.Question) {
		ask.Refused = fmt.Sprintf("Keep the question under %d characters.", sqlguess.MaxQuestionLen)
		return nil
	}

	cached, err := h.repo.FindConsoleQuestion(ctx, ask.Question)
	if err != nil {
		return err
	}
	if cached != nil {
		ask.Answered, ask.Cached = true, true
		ask.Explain, ask.Chart = cached.Explain, sqlguess.ParseChart(cached.Chart)
		data.SQL = cached.SQL
		l.Info("console question from history")
		return h.askRun(ctx, data)
	}

	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanAsk, h.ocrCfg.AskQuestionsPerMonth, time.Now())
	if err != nil {
		return err
	}
	if left <= 0 {
		ask.Refused = fmt.Sprintf("You've asked this month's %d questions. Until the 1st, rerun the ones in your history or write the SQL yourself.", limit)
		return nil
	}

	claims := getClaims(ctx)
	schema, err := h.repo.ConsoleSchema(ctx)
	if err != nil {
		return err
	}
	today := h.userToday(ctx)
	in := sqlguess.In{
		Question: ask.Question,
		Schema:   sqlguess.Condense(schema),
		Today:    today,
		Month:    claims.Period(today),
	}
	if claims != nil {
		in.DefaultCurrency = claims.DefaultCurrency.String
	}

	var usage gemini.Usage
	var out sqlguess.Out
	var guessErr error
	for try := 0; try < 2; try++ {
		gctx, cancel := context.WithTimeout(ctx, askTimeout)
		var u gemini.Usage
		out, u, guessErr = h.sqlGuess.Guess(gctx, in)
		cancel()
		usage = usage.Add(u)
		if guessErr != nil {
			break
		}
		out = sqlguess.Clean(out)
		ask.Answered = true
		ask.Explain, ask.Chart = out.Explain, out.Chart
		data.SQL, data.Err, data.Result = out.SQL, "", nil
		if out.SQL == "" {
			// the schema can't answer it: Explain tells why
			break
		}
		if len(out.SQL) > models.ConsoleMaxSQL {
			data.Err = "The query written is too long."
		} else if err := h.askRun(ctx, data); err != nil {
			return err
		}
		if data.Err == "" {
			break
		}
		in.Retry = &sqlguess.Retry{SQL: out.SQL, Error: data.Err}
	}

	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanAsk,
		Model:         h.sqlGuess.Model(),
		InputTokens:   usage.InputTokens,
		OutputTokens:  usage.OutputTokens,
		SearchQueries: usage.SearchQueries,
		CostMicros:    usage.CostMicros(h.ocrCfg.Prices()),
		OK:            guessErr == nil,
	}
	if guessErr != nil {
		scan.Error = guessErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a console question call", zap.Error(err))
	}
	if guessErr != nil {
		l.Error("writing a console question's sql", zap.Error(guessErr))
		ask.Answered = false
		ask.Refused = "Couldn't write that query just now, give it another try in a moment."
		return nil
	}
	l.Info("console question", zap.Bool("answered", out.SQL != ""), zap.Bool("retried", in.Retry != nil), zap.Bool("ok", data.Err == ""))

	if out.SQL != "" && data.Err == "" {
		if err := h.repo.SaveConsoleQuestion(ctx, models.ConsoleQuestion{
			Question: ask.Question, SQL: out.SQL, Explain: out.Explain, Chart: string(out.Chart),
		}); err != nil {
			l.Error("saving a console question", zap.Error(err))
		}
	}
	return nil
}

// askRun runs data.SQL in the console's sandbox into data's result or error.
func (h *Handler) askRun(ctx context.Context, data *views.ConsoleData) error {
	data.Query = true
	res, err := h.repo.ConsoleRun(ctx, data.SQL)
	switch {
	case errors.Is(err, repository.ErrTenantLocked), errors.Is(err, repository.ErrTenantDeleted):
		return err
	case err != nil:
		data.Err = err.Error()
	default:
		data.Result = &res
	}
	return nil
}

// DeleteConsoleQuestion drops a question from the Ask tab's history,
// answering the tab again.
func (h *Handler) DeleteConsoleQuestion(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if h.sqlGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "Asking is off.")
	}
	if err := h.repo.DeleteConsoleQuestion(ctx, r.PathValue("id")); err != nil {
		return nil, err
	}
	r.URL.RawQuery = url.Values{"ask": {""}}.Encode()
	w.Header().Set("HX-Push-Url", "/console?ask=")
	return h.Console(w, r)
}
