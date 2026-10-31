package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/smsingest"
	"github.com/omareloui/tyche/internal/smsparser"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const (
	maxSMSBatch     = 100
	maxSMSBodyBytes = 1 << 20
)

// pendingSMSStatuses are the statuses of sms that can still become transactions.
var pendingSMSStatuses = []models.SMSStatus{
	models.SMSStatusUnparsed, models.SMSStatusUnmatched, models.SMSStatusFailed,
}

type smsIn struct {
	Sender     string    `json:"sender"`
	Body       string    `json:"body"`
	ReceivedAt time.Time `json:"received_at"`
}

type ingestSMSReq struct {
	Messages []smsIn `json:"messages"`
}

type smsResult struct {
	Status        models.SMSStatus `json:"status"`
	TransactionID string           `json:"transaction_id,omitempty"`
	Error         string           `json:"error,omitempty"`
}

type ingestSMSResp struct {
	Results []smsResult `json:"results"`
}

// smsContext holds what's needed to turn an sms into a transaction, loaded
// once per request.
type smsContext struct {
	accounts       []models.Account
	counterparties []models.Counterparty
	// registry has the user's templates then the shared ones.
	registry *smsparser.Registry
	// loc is the time zone of the dates in the user's sms.
	loc *time.Location
}

func (h *Handler) IngestSMS(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	var req ingestSMSReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSMSBodyBytes)).Decode(&req); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid json body")
	}
	if len(req.Messages) > maxSMSBatch {
		return nil, resperr.New(http.StatusRequestEntityTooLarge, "too many messages")
	}

	sctx, err := h.loadSMSContext(ctx)
	if err != nil {
		return nil, err
	}

	deviceID, _ := ctx.Value(middleware.DeviceIDCtxKey{}).(string)

	resp := ingestSMSResp{Results: make([]smsResult, len(req.Messages))}
	for i, in := range req.Messages {
		res, err := h.ingestSMS(ctx, sctx, in, deviceID)
		if err != nil {
			// fail the request so the device retries the whole batch, the
			// already stored messages will be returned as duplicates.
			l.Error("ingesting sms", zap.Error(err), zap.Int("index", i))
			return nil, err
		}
		resp.Results[i] = *res
	}

	return json.Marshal(resp)
}

func (h *Handler) ingestSMS(ctx context.Context, sctx *smsContext, in smsIn, deviceID string) (*smsResult, error) {
	in.Sender = strings.TrimSpace(in.Sender)
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return &smsResult{Status: models.SMSStatusUnparsed, Error: "empty body"}, nil
	}
	if in.ReceivedAt.IsZero() {
		in.ReceivedAt = time.Now()
	}

	hash := smsingest.Hash(in.Sender, in.Body, in.ReceivedAt)

	existing, err := h.repo.GetSMSMessageByHash(ctx, hash)
	if err == nil {
		return &smsResult{Status: models.SMSStatusDuplicate, TransactionID: existing.TransactionID}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	msg := &models.SMSMessage{
		ID:         idgen.New(),
		Sender:     in.Sender,
		Body:       in.Body,
		ReceivedAt: in.ReceivedAt,
		Hash:       hash,
		DeviceID:   deviceID,
	}

	tx := h.processSMS(ctx, sctx, msg)

	if err := h.repo.CreateSMSMessage(ctx, msg, tx); err != nil {
		return nil, err
	}

	return &smsResult{Status: msg.Status, TransactionID: msg.TransactionID, Error: msg.Error}, nil
}

// processSMS parses the message and builds its transaction, setting the
// message status. The transaction is nil if it couldn't be built.
func (h *Handler) processSMS(ctx context.Context, sctx *smsContext, msg *models.SMSMessage) *models.Transaction {
	l := logger.FromCtx(ctx)

	m, err := sctx.registry.Parse(msg.Sender, msg.Body)
	msg.TemplateID = ""
	if m != nil {
		msg.TemplateID = m.Template.ID
	}
	if errors.Is(err, smsparser.ErrNotTransaction) {
		msg.Status, msg.Error = models.SMSStatusIgnored, ""
		return nil
	}
	if err != nil {
		msg.Status, msg.Error = models.SMSStatusUnparsed, err.Error()
		return nil
	}

	tx, err := smsingest.BuildTransaction(m.SMS, sctx.accounts, sctx.counterparties, msg.ReceivedAt, sctx.loc, fxrate.GetFxRate)
	if err != nil {
		msg.Status, msg.Error = models.SMSStatusFailed, err.Error()
		if errors.Is(err, smsingest.ErrUnmatchedAccount) {
			msg.Status = models.SMSStatusUnmatched
		}
		return nil
	}

	newCp := tx.Counterparty
	if newCp != nil && newCp.ID == "" {
		newCp.ID = idgen.New()
		if err := newCp.SanitizeAndValidate(h.sani, h.valid); err != nil {
			l.Warn("invalid counterparty from sms", zap.Error(err), zap.Any("counterparty", newCp))
			msg.Status, msg.Error = models.SMSStatusFailed, "invalid counterparty"
			return nil
		}
		if newCp.IsMerchant {
			newCp.Icon = brandicons.Default().Detect(newCp.Name, newCp.Aliases)
		}
		newCp.Color = brandicons.Default().ColorFor(newCp.Icon, "")
	} else {
		newCp = nil
	}

	if err := tx.SanitizeAndValidate(h.sani, h.valid); err != nil {
		l.Warn("invalid transaction from sms", zap.Error(err), zap.Any("transaction", tx))
		msg.Status, msg.Error = models.SMSStatusFailed, "invalid transaction"
		return nil
	}

	if newCp != nil {
		if err := h.repo.CreateCounterparty(ctx, newCp); err != nil {
			l.Error("creating counterparty from sms", zap.Error(err), zap.Any("counterparty", newCp))
			msg.Status, msg.Error = models.SMSStatusFailed, "creating counterparty: "+err.Error()
			return nil
		}
		l.Info("created counterparty from sms", zap.String("name", newCp.Name))
		// later messages in the same batch match it instead of creating it again
		sctx.counterparties = append(sctx.counterparties, *newCp)
	}

	msg.Status, msg.Error = models.SMSStatusParsed, ""
	return tx
}

func (h *Handler) SMSPage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	msgs, err := h.repo.GetSMSMessagesByStatus(ctx, pendingSMSStatuses...)
	if err != nil {
		return nil, err
	}

	devices, err := h.repo.GetDevices(ctx)
	if err != nil {
		return nil, err
	}

	loc, err := h.repo.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}

	templates, err := h.repo.GetSMSTemplates(ctx)
	if err != nil {
		return nil, err
	}

	return views.SMS(getClaims(ctx), b, msgs, devices, views.NewTimezoneFD(loc), len(templates)), nil
}

// SetTimezone sets the time zone of the dates in the user's sms.
func (h *Handler) SetTimezone(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	fd := &views.TimezoneFD{Name: strings.TrimSpace(r.FormValue("timezone"))}

	loc, err := models.LoadTimezone(fd.Name)
	if err != nil {
		fd.Error = "Unknown time zone, use a name like Africa/Cairo"
		return views.TimezoneCard(fd), nil
	}
	if err := h.repo.SetTimezone(r.Context(), loc); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("set the sms time zone", zap.String("timezone", loc.String()))

	fd = views.NewTimezoneFD(loc)
	fd.Saved = true
	return views.TimezoneCard(fd), nil
}

func (h *Handler) RetrySMS(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	msg, err := h.getPendingSMS(r)
	if err != nil {
		return nil, err
	}

	sctx, err := h.loadSMSContext(ctx)
	if err != nil {
		return nil, err
	}

	tx := h.processSMS(ctx, sctx, msg)
	if err := h.repo.UpdateSMSMessageResult(ctx, msg, tx); err != nil {
		return nil, err
	}
	l.Info("retried sms", zap.String("id", msg.ID), zap.String("status", string(msg.Status)))

	if tx != nil {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return views.SMSMessage(msg), nil
}

// RetryAllSMS re-processes every pending sms and returns the ones still pending.
func (h *Handler) RetryAllSMS(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	msgs, err := h.repo.GetSMSMessagesByStatus(ctx, pendingSMSStatuses...)
	if err != nil {
		return nil, err
	}

	sctx, err := h.loadSMSContext(ctx)
	if err != nil {
		return nil, err
	}

	stillPending := []models.SMSMessage{}
	created := 0
	for i := range msgs {
		msg := &msgs[i]
		tx := h.processSMS(ctx, sctx, msg)
		if err := h.repo.UpdateSMSMessageResult(ctx, msg, tx); err != nil {
			return nil, err
		}
		if tx != nil {
			created++
		}
		if slices.Contains(pendingSMSStatuses, msg.Status) {
			stillPending = append(stillPending, *msg)
		}
	}
	l.Info("retried all sms", zap.Int("count", len(msgs)), zap.Int("transactions_created", created))

	if created > 0 {
		if err := h.getAndRenderBalancesOOB(w, r); err != nil {
			return nil, err
		}
	}

	return views.SMSList(stillPending), nil
}

func (h *Handler) IgnoreSMS(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	msg, err := h.getPendingSMS(r)
	if err != nil {
		return nil, err
	}

	msg.Status = models.SMSStatusIgnored
	if err := h.repo.UpdateSMSMessageResult(r.Context(), msg, nil); err != nil {
		return nil, err
	}

	// removes the card
	return nil, nil
}

func (h *Handler) RevokeDevice(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	if err := h.repo.RevokeDevice(r.Context(), id); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("revoked device", zap.String("id", id))

	devices, err := h.repo.GetDevices(r.Context())
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.ID == id {
			return views.Device(&d), nil
		}
	}
	return nil, nil
}

// getPendingSMS gets the sms in the path if it didn't become a transaction
// nor got ignored.
func (h *Handler) getPendingSMS(r *http.Request) (*models.SMSMessage, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	msg, err := h.repo.GetSMSMessage(r.Context(), id)
	if err != nil {
		return nil, err
	}

	if msg.Status == models.SMSStatusParsed || msg.Status == models.SMSStatusIgnored {
		return nil, resperr.New(http.StatusConflict, "sms is already "+string(msg.Status))
	}
	return msg, nil
}

func (h *Handler) loadSMSContext(ctx context.Context) (*smsContext, error) {
	accounts, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	counterparties, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		return nil, err
	}
	registry, err := h.smsRegistry(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := h.repo.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	return &smsContext{accounts: accounts, counterparties: counterparties, registry: registry, loc: loc}, nil
}

// smsRegistry has the user's templates, tried first, then the shared ones.
func (h *Handler) smsRegistry(ctx context.Context) (*smsparser.Registry, error) {
	mine, err := h.repo.GetSMSTemplates(ctx)
	if err != nil {
		return nil, err
	}
	r := smsparser.NewRegistry(mine, h.smsTemplates.List())
	for _, e := range r.Errors() {
		logger.FromCtx(ctx).Warn("skipping an invalid sms template", zap.String("id", e.Template.ID), zap.String("scope", string(e.Template.Scope)), zap.Error(e.Err))
	}
	return r, nil
}

// VerifyDevice is used by the device auth middleware.
func (h *Handler) VerifyDevice(ctx context.Context, deviceID, tokenID string) error {
	return h.repo.VerifyDevice(ctx, deviceID, tokenID)
}
