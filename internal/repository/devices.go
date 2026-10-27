package repository

import (
	"context"

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

	if err := q.CreateDevice(ctx, queries.CreateDeviceParams{ID: d.ID, Name: d.Name}); err != nil {
		l.Error("creating device", zap.Error(err), zap.Any("device", d))
		return err
	}
	l.Debug("created device", zap.Any("device", d))
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

// VerifyDevice errors if the device doesn't exist or got revoked, and marks
// it as seen otherwise.
func (r *Repository) VerifyDevice(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if _, err := q.GetActiveDevice(ctx, id); err != nil {
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
		LastSeenAt: d.LastSeenAt.Time,
		RevokedAt:  d.RevokedAt.Time,
		CreatedAt:  d.CreatedAt,
	}
}
