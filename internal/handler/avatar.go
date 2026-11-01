package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/avatar"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

// Avatar serves the user's downloaded picture, its url is versioned by its
// hash so it's cached for good.
func (h *Handler) Avatar(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	pic, err := h.repo.GetUserPicture(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "no picture")
	}
	if err != nil {
		return nil, err
	}

	etag := strconv.Quote(pic.Hash)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil, nil
	}
	w.Header().Set("Content-Type", pic.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(pic.Data)))
	_, err = w.Write(pic.Data)
	return nil, err
}

// syncPicture downloads the user's picture at url when it's new or wasn't
// downloaded yet, keeping the provider's url when it fails.
func (h *Handler) syncPicture(ctx context.Context, usr *models.User, url string) {
	if url == "" || (url == usr.Picture && usr.PictureHash != "") {
		return
	}
	l := logger.FromCtx(ctx)

	pic, err := avatar.Fetch(ctx, url)
	if err != nil {
		l.Warn("downloading the profile picture", zap.Error(err))
		return
	}
	if err := h.repo.SetUserPicture(ctx, url, pic); err != nil {
		l.Warn("storing the profile picture", zap.Error(err))
		return
	}
	usr.Picture = url
	usr.PictureHash = pic.Hash
}
