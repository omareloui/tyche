package handler

import (
	"cmp"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/smsingest"
	"github.com/omareloui/tyche/internal/smsparser"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// SMSTemplates lists the user's templates and the shared ones they can
// override with their own.
func (h *Handler) SMSTemplates(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("fetching balances", zap.Error(err))
		return nil, err
	}

	mine, err := h.repo.GetSMSTemplates(ctx)
	if err != nil {
		return nil, err
	}

	return views.SMSTemplates(getClaims(ctx), b, views.SMSTemplateLists{
		Mine:   views.NewSMSTemplateRows(mine),
		Shared: views.NewSMSTemplateRows(h.smsTemplates.List()),
	}, r.URL.Query().Get("sms")), nil
}

// SMSTemplateSandbox reads an sms with every template, as an ingested one
// would be, and builds its transaction without saving anything.
func (h *Handler) SMSTemplateSandbox(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	res := &views.SMSSandboxResult{
		Sender:  strings.TrimSpace(r.FormValue("sender")),
		Body:    strings.TrimSpace(r.FormValue("body")),
		IsAdmin: getClaims(ctx).IsAdmin(),
	}
	if res.Body == "" {
		return views.SMSSandbox(res), nil
	}

	sctx, err := h.loadSMSContext(ctx)
	if err != nil {
		return nil, err
	}
	res.Broken = sctx.registry.Errors()

	m, err := sctx.registry.Parse(res.Sender, res.Body)
	res.Match = m
	switch {
	case errors.Is(err, smsparser.ErrNotTransaction):
		res.Ignored = true
	case err != nil:
		res.Err = err.Error()
	default:
		tx, err := smsingest.BuildTransaction(m.SMS, sctx.accounts, sctx.counterparties, time.Now(), sctx.loc, fxrate.GetFxRate)
		if err != nil {
			res.TxErr = err.Error()
		} else {
			res.Tx = tx
		}
	}

	return views.SMSSandbox(res), nil
}

func (h *Handler) NewSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if smsTemplateFormPage(w, r) {
		return nil, nil
	}

	q := r.URL.Query()
	scope, ok := models.ParseSMSTemplateScope(q.Get("scope"))
	if !ok || !h.mayWriteShared(r) {
		scope = models.SMSTemplateMine
	}

	var t *models.SMSTemplate
	if id := q.Get("copy"); id != "" {
		// overriding a shared template: the user's ones are tried first, and
		// a higher priority tries it before the original among its scope
		shared, err := h.smsTemplates.Get(id)
		if err != nil {
			return nil, resperr.New(http.StatusNotFound, "sms template not found")
		}
		t = shared
		t.ID, t.SeedKey, t.Name = "", "", t.Name+" (mine)"
		t.Priority++
	} else {
		sample := strings.TrimSpace(q.Get("sms"))
		sender := strings.TrimSpace(q.Get("sender"))
		t = &models.SMSTemplate{
			Bank:      cmp.Or(strings.TrimSpace(q.Get("bank")), h.bankOfSender(sender)),
			Action:    models.SMSTemplateTransaction,
			TxKind:    models.ExpenseTxKind,
			Senders:   views.SplitSenders(sender),
			Body:      sample,
			Sample:    sample,
			IsEnabled: true,
		}
	}
	t.Scope = scope

	fd := views.NewSMSTemplateFD(t)
	fd.IsNew = true
	fd.IsAdmin = getClaims(ctx).IsAdmin()

	return views.SMSTemplateDrawer(fd, h.previewSMSTemplate(fd)), nil
}

func (h *Handler) EditSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if smsTemplateFormPage(w, r) {
		return nil, nil
	}

	t, err := h.getSMSTemplate(r)
	if err != nil {
		return nil, err
	}
	fd := views.NewSMSTemplateFD(t)
	fd.IsAdmin = getClaims(r.Context()).IsAdmin()

	return views.SMSTemplateDrawer(fd, h.previewSMSTemplate(fd)), nil
}

// smsTemplateFormPage sends a navigation to a form drawer (a link opened in
// a new tab, an old bookmark) to the templates page opening it, telling
// whether it did.
func smsTemplateFormPage(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Boosted") == "" {
		return false
	}
	page := views.SMSTemplatesFormURL(r.URL.RequestURI())
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", page)
		return true
	}
	http.Redirect(w, r, page, http.StatusSeeOther)
	return true
}

// smsTemplateSaved closes the drawer of a saved template, answering its
// scope's list again.
func (h *Handler) smsTemplateSaved(w http.ResponseWriter, r *http.Request, scope models.SMSTemplateScope) (templ.Component, error) {
	ctx := r.Context()
	var ts []models.SMSTemplate
	if scope == models.SMSTemplateShared {
		ts = h.smsTemplates.List()
	} else {
		var err error
		if ts, err = h.repo.GetSMSTemplates(ctx); err != nil {
			return nil, err
		}
	}
	drawerSaved(w, "#"+views.SMSTemplateSectionID(scope), "outerHTML")
	return views.SMSTemplateSection(scope, getClaims(ctx).IsAdmin(), views.NewSMSTemplateRows(ts)), nil
}

// PreviewSMSTemplate compiles the template in the form and reads its sample.
func (h *Handler) PreviewSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	fd := parseSMSTemplateForm(r, models.SMSTemplateMine)
	return views.SMSTemplatePreview(h.previewSMSTemplate(fd)), nil
}

func (h *Handler) CreateSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	scope, ok := models.ParseSMSTemplateScope(r.FormValue("scope"))
	if !ok {
		return nil, resperr.New(http.StatusBadRequest, "invalid scope")
	}
	if scope == models.SMSTemplateShared && !h.mayWriteShared(r) {
		l.Warn("non admin trying to create a shared sms template")
		return nil, resperr.New(http.StatusNotFound, "not found")
	}
	fd := parseSMSTemplateForm(r, scope)
	fd.IsNew = true

	t := fd.Template()
	t.ID = idgen.New()
	if !h.validateSMSTemplate(t, fd) {
		return views.SMSTemplateForm(fd, h.previewSMSTemplate(fd)), nil
	}

	var err error
	if scope == models.SMSTemplateShared {
		err = h.smsTemplates.Create(ctx, t)
	} else {
		err = h.repo.CreateSMSTemplate(ctx, t)
	}
	if err != nil {
		l.Error("creating sms template", zap.Error(err), zap.String("scope", string(scope)))
		return nil, err
	}
	l.Info("created sms template", zap.String("id", t.ID), zap.String("scope", string(scope)))

	return h.smsTemplateSaved(w, r, scope)
}

func (h *Handler) UpdateSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	prev, err := h.getSMSTemplate(r)
	if err != nil {
		return nil, err
	}

	fd := parseSMSTemplateForm(r, prev.Scope)
	fd.ID = prev.ID
	t := fd.Template()
	if !h.validateSMSTemplate(t, fd) {
		return views.SMSTemplateForm(fd, h.previewSMSTemplate(fd)), nil
	}

	if prev.Scope == models.SMSTemplateShared {
		err = h.smsTemplates.Update(ctx, t)
	} else {
		err = h.repo.UpdateSMSTemplate(ctx, t)
	}
	if err != nil {
		l.Error("updating sms template", zap.Error(err), zap.String("id", t.ID))
		return nil, err
	}
	l.Info("updated sms template", zap.String("id", t.ID), zap.String("scope", string(t.Scope)))

	return h.smsTemplateSaved(w, r, prev.Scope)
}

func (h *Handler) DeleteSMSTemplate(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	t, err := h.getSMSTemplate(r)
	if err != nil {
		return nil, err
	}

	if t.Scope == models.SMSTemplateShared {
		err = h.smsTemplates.Delete(ctx, t.ID)
	} else {
		err = h.repo.DeleteSMSTemplate(ctx, t.ID)
	}
	if err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("deleted sms template", zap.String("id", t.ID), zap.String("scope", string(t.Scope)))

	// removes the row
	return nil, nil
}

// getSMSTemplate gets the template in the path to edit or delete it, the
// shared ones only for admins.
func (h *Handler) getSMSTemplate(r *http.Request) (*models.SMSTemplate, error) {
	scope, ok := models.ParseSMSTemplateScope(r.PathValue("scope"))
	id := r.PathValue("id")
	if !ok || id == "" {
		return nil, errs.ErrInvalidID
	}
	if scope == models.SMSTemplateShared && !h.mayWriteShared(r) {
		logger.FromCtx(r.Context()).Warn("non admin trying to change a shared sms template", zap.String("id", id))
		return nil, resperr.New(http.StatusNotFound, "sms template not found")
	}

	var t *models.SMSTemplate
	var err error
	if scope == models.SMSTemplateShared {
		t, err = h.smsTemplates.Get(id)
	} else {
		t, err = h.repo.GetSMSTemplate(r.Context(), id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "sms template not found")
	}
	return t, err
}

// mayWriteShared is whether the user can change the shared templates: admins,
// with HX-Request on unsafe methods like middleware.Admin.
func (h *Handler) mayWriteShared(r *http.Request) bool {
	isSafe := r.Method == http.MethodGet || r.Method == http.MethodHead
	return getClaims(r.Context()).IsAdmin() && (isSafe || r.Header.Get("HX-Request") == "true")
}

// bankOfSender is the bank of the shared templates for the sender, else the
// sender itself.
func (h *Handler) bankOfSender(sender string) string {
	if sender == "" {
		return ""
	}
	for _, t := range h.smsTemplates.List() {
		if t.Bank != "" && t.AppliesTo(sender) {
			return t.Bank
		}
	}
	return sender
}

// validateSMSTemplate sets the form errors, false if there are any.
func (h *Handler) validateSMSTemplate(t *models.SMSTemplate, fd *views.SMSTemplateFD) bool {
	if fd.Errors == nil {
		fd.Errors = map[string]string{}
	}

	if err := t.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if !errors.As(err, &valerr) {
			fd.Errors["Name"] = err.Error()
			return false
		}
		for k, v := range valerr.Errors {
			fd.Errors[k] = v.String()
		}
	}

	if _, err := smsparser.Compile(t.Body); err != nil && fd.Errors["Body"] == "" {
		fd.Errors["Body"] = err.Error()
	}

	if t.Sample != "" && fd.Errors["Body"] == "" {
		if _, err := smsparser.MatchTemplate(*t, t.Sample); err != nil && !(t.Action == models.SMSTemplateIgnore && errors.Is(err, smsparser.ErrNotTransaction)) {
			fd.Errors["Sample"] = "The template doesn't read it: " + err.Error()
		}
	}

	return len(fd.Errors) == 0
}

func (h *Handler) previewSMSTemplate(fd *views.SMSTemplateFD) *views.SMSTemplatePreviewData {
	p := &views.SMSTemplatePreviewData{HasSample: strings.TrimSpace(fd.Sample) != ""}
	t := fd.Template()

	pattern, err := smsparser.Compile(t.Body)
	if err != nil {
		p.CompileErr = err.Error()
		return p
	}
	p.Fields = pattern.Fields()
	if !p.HasSample {
		return p
	}

	m, err := smsparser.MatchTemplate(*t, t.Sample)
	p.Match = m
	switch {
	case errors.Is(err, smsparser.ErrNotTransaction):
		p.Ignored = true
	case err != nil:
		p.MatchErr = err.Error()
	}
	return p
}

func parseSMSTemplateForm(r *http.Request, scope models.SMSTemplateScope) *views.SMSTemplateFD {
	fd := &views.SMSTemplateFD{
		Scope:     scope,
		Name:      r.FormValue("name"),
		Bank:      r.FormValue("bank"),
		Senders:   r.FormValue("senders"),
		Action:    models.SMSTemplateAction(r.FormValue("action")),
		TxKind:    r.FormValue("tx_kind"),
		Body:      r.FormValue("body"),
		Sample:    r.FormValue("sample"),
		Priority:  r.FormValue("priority"),
		IsEnabled: r.FormValue("is_enabled") != "",
		Fixed:     map[string]string{},
		IsAdmin:   getClaims(r.Context()).IsAdmin(),
	}
	for _, f := range models.SMSTemplateFixedFields {
		if v := strings.TrimSpace(r.FormValue("fixed_" + f)); v != "" {
			fd.Fixed[f] = v
		}
	}
	if _, err := strconv.Atoi(strings.TrimSpace(fd.Priority)); fd.Priority != "" && err != nil {
		fd.Errors = map[string]string{"Priority": "Has to be a whole number"}
	}
	return fd
}
