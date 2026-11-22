package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/receiptimg"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// ItemImage serves an item's picture, its url versioned by its hash so it's
// cached for good.
func (h Handler) ItemImage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	img, err := h.repo.GetItemImage(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "No picture")
	}
	if err != nil {
		return nil, err
	}
	etag := strconv.Quote(img.Hash)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil, nil
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(img.Data)))
	_, err = w.Write(img.Data)
	return nil, err
}

// SetItemImage sets the item's picture from the posted "image" (shrunk into
// a small WebP, receiptimg.Thumbnail), answering its details.
func (h Handler) SetItemImage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	slot := "#" + views.ItemImageErrorID

	data, ct, err := readImageUpload(w, r, "image")
	if err == nil && data == nil {
		err = errScan{"Pick a photo of the item."}
	}
	if err != nil {
		return nil, scanError(w, slot, err)
	}
	th, err := receiptimg.Thumbnail(data, ct)
	if err != nil {
		l.Warn("converting an item's picture", zap.Error(err), zap.String("id", it.Item.ID), zap.String("type", ct))
		return nil, scanError(w, slot, errScan{"Couldn't read that image, try another one."})
	}
	if err := h.repo.SetItemImage(ctx, it.Item.ID, th.Data, th.Width, th.Height); err != nil {
		return nil, err
	}
	l.Info("set an item's picture", zap.String("id", it.Item.ID), zap.Int("from_bytes", len(data)), zap.Int("bytes", len(th.Data)))

	addHXTrigger(w, views.ItemsChangedEvent)
	return h.answerItemDetails(w, r, it.Item.ID)
}

// DeleteItemImage deletes the item's picture, answering its details.
func (h Handler) DeleteItemImage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.DeleteItemImage(r.Context(), it.Item.ID); err != nil {
		logger.FromCtx(r.Context()).Error("deleting an item's picture", zap.Error(err), zap.String("id", it.Item.ID))
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("deleted an item's picture", zap.String("id", it.Item.ID))

	addHXTrigger(w, views.ItemsChangedEvent)
	return h.answerItemDetails(w, r, it.Item.ID)
}
