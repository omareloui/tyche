package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetUserByOAuthID(ctx context.Context, oauthId string) (*models.User, error) {
	l := logger.FromCtx(ctx)
	dbU, err := r.q.GetUserByOAuthID(ctx, oauthId)
	if err != nil {
		l.Error("fetching user", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched user")
	return dbUserToUser(dbU), nil
}

func (r *Repository) CreateUser(ctx context.Context, usr *models.User) error {
	return r.q.CreateUser(ctx, db.CreateUserParams{
		ID:                usr.ID,
		OauthID:           usr.OAuthID,
		OauthProvider:     int64(usr.OAuthProvider),
		Email:             usr.Email,
		VerifiedEmail:     usr.VerifiedEmail,
		FirstName:         usr.FirstName,
		LastName:          usr.LastName,
		Picture:           sql.NullString{String: usr.Picture, Valid: usr.Picture != ""},
		DefaultCurrencyID: usr.DefaultCurrency.ID,
		StartOfPeriod:     int64(usr.StartOfPeriod),
	})
}

func dbUserToUser(dbU db.GetUserByOAuthIDRow) *models.User {
	return &models.User{
		ID:            dbU.User.ID,
		OAuthID:       dbU.User.OauthID,
		OAuthProvider: models.OAuthProvider(dbU.User.OauthProvider),
		Email:         dbU.User.Email,
		VerifiedEmail: dbU.User.VerifiedEmail,
		FirstName:     dbU.User.FirstName,
		LastName:      dbU.User.LastName,
		Picture:       dbU.User.Picture.String,
		StartOfPeriod: uint8(dbU.User.StartOfPeriod),
		DefaultCurrency: &models.Currency{
			ID:   dbU.Currency.ID,
			Enum: models.CurrencyEnum(dbU.Currency.Enum),
		},
	}
}
