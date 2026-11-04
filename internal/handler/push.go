package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/omareloui/tyche/internal/badge"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
)

// The installed apps' badge (internal/badge, web/public/js/badge.js): each
// one subscribes to Web Push with the counts it shows, set on its own
// settings page.

const maxPushBody = 8 << 10

// RefreshBadge counts the badge again after a request (middleware.Badge).
func (h *Handler) RefreshBadge(ctx context.Context, tenantID string, write bool) {
	h.badges.AfterRequest(ctx, tenantID, write)
}

type pushKeyResp struct {
	Key string `json:"key"`
}

// PushKey is the VAPID key to subscribe with, 404 when pushes are off.
func (h *Handler) PushKey(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if !h.badges.Enabled() {
		return nil, resperr.New(http.StatusNotFound, "Notifications aren't set up.")
	}
	return json.Marshal(pushKeyResp{Key: h.badges.PublicKey()})
}

type pushSubscriptionReq struct {
	Subscription webpush.Subscription `json:"subscription"`
	Sources      []string             `json:"sources"`
}

// Subscribe saves the browser's push subscription with the counts it shows.
func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if !h.badges.Enabled() {
		return nil, resperr.New(http.StatusNotFound, "Notifications aren't set up.")
	}
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	var req pushSubscriptionReq
	err = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBody)).Decode(&req)
	sub := req.Subscription
	if err != nil || !strings.HasPrefix(sub.Endpoint, "https://") || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		return nil, resperr.New(http.StatusBadRequest, "Invalid subscription.")
	}
	srcs := badge.ParseSources(strings.Join(req.Sources, ","))
	if len(srcs) == 0 {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick what the badge counts.")
	}
	if err := h.badges.Subscribe(r.Context(), tenantID, sub, srcs); err != nil {
		return nil, err
	}
	return []byte("{}"), nil
}

type pushUnsubscribeReq struct {
	Endpoint string `json:"endpoint"`
}

// Unsubscribe drops the browser's push subscription.
func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if !h.badges.Enabled() {
		return nil, resperr.New(http.StatusNotFound, "Notifications aren't set up.")
	}
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	var req pushUnsubscribeReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBody)).Decode(&req); err != nil || req.Endpoint == "" {
		return nil, resperr.New(http.StatusBadRequest, "Invalid subscription.")
	}
	if err := h.badges.Unsubscribe(r.Context(), tenantID, req.Endpoint); err != nil {
		return nil, err
	}
	return []byte("{}"), nil
}

// BadgeCounts are the badge's counts, for the app to set its badge while
// it's open.
func (h *Handler) BadgeCounts(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	c, err := h.repo.GetBadgeCounts(r.Context())
	if err != nil {
		return nil, err
	}
	w.Header().Set("Cache-Control", "no-store")
	return json.Marshal(c)
}
