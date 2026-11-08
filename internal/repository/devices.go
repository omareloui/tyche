package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) CreateDevice(ctx context.Context, d *models.Device) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.CreateDevice(ctx, queries.CreateDeviceParams{
		ID:        d.ID,
		Name:      d.Name,
		InstallID: nullString(d.InstallID),
		TokenID:   nullString(d.TokenID),
	}); err != nil {
		l.Error("creating device", zap.Error(err), zap.String("id", d.ID))
		return err
	}
	l.Debug("created device", zap.String("id", d.ID))
	return nil
}

// RegisterDevice creates the device, or if its install already has a device,
// reuses it: sets d.ID to it, renames it, un-revokes it, and replaces its
// token id so older tokens stop working.
func (r *Repository) RegisterDevice(ctx context.Context, d *models.Device) error {
	if d.InstallID == "" {
		return r.CreateDevice(ctx, d)
	}

	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	id, err := q.GetDeviceIDByInstallID(ctx, nullString(d.InstallID))
	if errors.Is(err, sql.ErrNoRows) {
		return r.CreateDevice(ctx, d)
	}
	if err != nil {
		l.Error("fetching device by install id", zap.Error(err))
		return err
	}

	d.ID = id
	if err := q.RenewDevice(ctx, queries.RenewDeviceParams{
		ID:      d.ID,
		Name:    d.Name,
		TokenID: nullString(d.TokenID),
	}); err != nil {
		l.Error("renewing device", zap.Error(err), zap.String("id", d.ID))
		return err
	}
	l.Debug("renewed device", zap.String("id", d.ID))
	return nil
}

func (r *Repository) GetDevices(ctx context.Context) ([]models.Device, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbDevices, err := q.GetDevices(ctx)
	if err != nil {
		l.Error("fetching devices", zap.Error(err))
		return nil, err
	}

	devices := make([]models.Device, len(dbDevices))
	for i, d := range dbDevices {
		devices[i] = *dbDeviceToDevice(&d.Device)
	}
	return devices, nil
}

// VerifyDevice errors if the device doesn't exist, got revoked, or tokenID
// isn't its current token, and marks it as seen otherwise.
func (r *Repository) VerifyDevice(ctx context.Context, id, tokenID string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	// Valid even if empty, tokens of devices registered before token ids
	// have none, matching their NULL token id.
	if _, err := q.GetActiveDevice(ctx, queries.GetActiveDeviceParams{
		ID:      id,
		TokenID: sql.NullString{String: tokenID, Valid: true},
	}); err != nil {
		return err
	}

	if err := q.TouchDevice(ctx, id); err != nil {
		l := logger.FromCtx(ctx)
		l.Warn("updating device last seen", zap.Error(err), zap.String("id", id))
	}
	return nil
}

func (r *Repository) RevokeDevice(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.RevokeDevice(ctx, id); err != nil {
		l.Error("revoking device", zap.Error(err), zap.String("id", id))
		return err
	}
	l.Debug("revoked device", zap.String("id", id))
	return nil
}

func dbDeviceToDevice(d *queries.Device) *models.Device {
	return &models.Device{
		ID:         d.ID,
		Name:       d.Name,
		InstallID:  d.InstallID.String,
		TokenID:    d.TokenID.String,
		LastSeenAt: d.LastSeenAt.Time,
		RevokedAt:  d.RevokedAt.Time,
		CreatedAt:  d.CreatedAt,
	}
}
