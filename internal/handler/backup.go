package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// maxBackupBytes bounds an imported backup.
const maxBackupBytes = 200 << 20

func (h *Handler) BackupPage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return nil, err
	}
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	data := views.BackupData{
		Encrypted: h.vaults.IsEncrypted(tenantID),
		HasPrev:   h.repo.HasBackup(r.Context()),
	}
	return views.Backup(getClaims(r.Context()), b, data), nil
}

// ExportBackup downloads the user's database, in plain text.
func (h *Handler) ExportBackup(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	path, err := h.repo.ExportDB(r.Context())
	if err != nil {
		return nil, err
	}
	defer os.Remove(path)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	name := "tyche-backup-" + time.Now().Format("2006-01-02") + ".sqlite"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Cache-Control", "no-store")
	_, err = io.Copy(w, f)
	return nil, err
}

// ImportBackup replaces the user's data with an exported database, read from
// a multipart form with the confirmation then the file.
func (h *Handler) ImportBackup(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}

	var confirmed, imported bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, uploadErr(err)
		}
		switch part.FormName() {
		case "confirm":
			v, _ := io.ReadAll(io.LimitReader(part, 64))
			confirmed = strings.TrimSpace(string(v)) == models.ImportConfirmation
		case "file":
			if !confirmed {
				return nil, resperr.New(http.StatusUnprocessableEntity, "Type "+models.ImportConfirmation+" to confirm.")
			}
			if part.FileName() == "" {
				return nil, resperr.New(http.StatusUnprocessableEntity, "Choose a backup file.")
			}
			if err := h.repo.ImportDB(r.Context(), part); err != nil {
				return nil, uploadErr(err)
			}
			imported = true
		}
		part.Close()
	}
	if !confirmed {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type "+models.ImportConfirmation+" to confirm.")
	}
	if !imported {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Choose a backup file.")
	}

	return nil, h.afterReplace(w, r)
}

// RestorePrevBackup undoes the last import (or the last restore).
func (h *Handler) RestorePrevBackup(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if !h.repo.HasBackup(r.Context()) {
		return nil, resperr.New(http.StatusNotFound, "There's no previous data to restore.")
	}
	if err := h.repo.RestoreBackup(r.Context()); err != nil {
		return nil, err
	}
	return nil, h.afterReplace(w, r)
}

// afterReplace signs out the phones (their tokens were for the replaced
// database) and drops the access token, which carries the default currency
// and account.
func (h *Handler) afterReplace(w http.ResponseWriter, r *http.Request) error {
	tenantID, err := repository.TenantIDFromCtx(r.Context())
	if err != nil {
		return err
	}
	if err := h.vaults.DeleteDevices(r.Context(), tenantID); err != nil {
		return err
	}
	unsetCookie(w, accessTokenCookieName)
	w.Header().Set("HX-Redirect", "/")
	return nil
}

func uploadErr(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return resperr.New(http.StatusRequestEntityTooLarge, "The backup is too big (200 MB at most).")
	}
	var invalid *repository.InvalidBackupError
	if errors.As(err, &invalid) {
		return resperr.New(http.StatusUnprocessableEntity, invalid.Reason)
	}
	return err
}
