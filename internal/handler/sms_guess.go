package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/smsguess"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// smsGuessTimeout is how long guessing a template may take (two calls).
const smsGuessTimeout = 90 * time.Second

// SetSMSGuess turns guessing sms templates on (a nil guesser keeps it off).
func (h *Handler) SetSMSGuess(g smsguess.Guesser) {
	h.smsGuess = g
}

// GuessSMSTemplate has the AI guess the template of an unparsed sms, then
// opens the template form prefilled with it (the guess is kept on the sms,
// so opening it again is free). Nothing is saved as a template: the user
// does it, then retries the sms.
func (h *Handler) GuessSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if h.smsGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "Guessing templates is off")
	}
	ctx := r.Context()
	msg, err := h.getPendingSMS(r)
	if err != nil {
		return nil, err
	}
	slot := views.SMSGuessErrorSlot(msg.ID)
	if msg.Status != models.SMSStatusUnparsed {
		return nil, scanError(w, slot, errScan{"Only an SMS no template reads can be guessed."})
	}

	if msg.TemplateGuess == nil {
		guess, err := h.guessSMSTemplate(ctx, msg)
		if err != nil {
			return nil, scanError(w, slot, err)
		}
		if err := h.repo.SetSMSTemplateGuess(ctx, msg.ID, guess); err != nil {
			return nil, err
		}
	}

	w.Header().Set("HX-Redirect", views.SMSGuessFormURL(msg.ID))
	return nil, nil
}

// guessSMSTemplate asks the AI, within the tenant's monthly guesses, for a
// template verified to read the sms.
func (h *Handler) guessSMSTemplate(ctx context.Context, msg *models.SMSMessage) (*models.SMSTemplateGuess, error) {
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	limit := h.ocrCfg.SMSGuessesPerMonth
	left, err := h.repo.SMSGuessesLeft(ctx, tenantID, limit, time.Now())
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, errScan{fmt.Sprintf("You've had %d SMS guessed this month, the most you can. It starts again on the 1st.", limit)}
	}

	mine, err := h.repo.GetSMSTemplates(ctx)
	if err != nil {
		return nil, err
	}
	in := smsguess.In{
		Sender:   msg.Sender,
		Body:     msg.Body,
		Examples: smsguess.PickExamples(msg.Sender, append(mine, h.smsTemplates.List()...)),
	}

	guessCtx, cancel := context.WithTimeout(ctx, smsGuessTimeout)
	defer cancel()
	guess, usage, guessErr := smsguess.GuessVerified(guessCtx, h.smsGuess, in)
	scan := models.ReceiptScan{
		TenantID:     tenantID,
		Kind:         models.ScanSMSGuess,
		Model:        h.smsGuess.Model(),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostMicros:   usage.CostMicros(h.ocrCfg.InputPerM, h.ocrCfg.OutputPerM),
		OK:           guessErr == nil,
	}
	if guessErr != nil {
		scan.Error = guessErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording an sms guess", zap.Error(err))
	}

	if errors.Is(guessErr, smsguess.ErrNoGuess) {
		l.Info("no sms template guessed", zap.String("sms_id", msg.ID), zap.Error(guessErr))
		return nil, errScan{"Couldn't guess a template reading this SMS. Make one by hand instead."}
	}
	if guessErr != nil {
		l.Error("guessing an sms template", zap.String("sms_id", msg.ID), zap.Error(guessErr))
		return nil, errScan{"Couldn't guess a template right now, try again in a bit."}
	}
	l.Info("guessed an sms template", zap.String("sms_id", msg.ID), zap.String("action", string(guess.Action)), zap.Int("kind", int(guess.TxKind)))
	return guess, nil
}
