package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetUserByOAuthID(ctx context.Context, oauthId string) (*models.User, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbU, err := q.GetUserByOAuthID(ctx, oauthId)
	if err != nil {
		l.Error("fetching user", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched user")
	return dbUserRowToUser(dbU), nil
}

func (r *Repository) CreateUser(ctx context.Context, usr *models.User) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.CreateUser(ctx, queries.CreateUserParams{
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

func dbUserToUser(dbU *queries.User, dfltCur *queries.Currency) *models.User {
	u := &models.User{
		ID:            dbU.ID,
		OAuthID:       dbU.OauthID,
		OAuthProvider: models.OAuthProvider(dbU.OauthProvider),
		Email:         dbU.Email,
		VerifiedEmail: dbU.VerifiedEmail,
		FirstName:     dbU.FirstName,
		LastName:      dbU.LastName,
		Picture:       dbU.Picture.String,
		StartOfPeriod: uint8(dbU.StartOfPeriod),
		DefaultCurrency: models.Currency{
			ID: dbU.DefaultCurrencyID,
		},
	}

	if dfltCur != nil {
		u.DefaultCurrency = *dbCurrencyToCurrency(dfltCur)
	}

	return u
}

func dbUserRowToUser(dbU queries.GetUserByOAuthIDRow) *models.User {
	return dbUserToUser(&dbU.User, &dbU.Currency)
}
