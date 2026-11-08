package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/avatar"
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

	if usr.StartOfPeriod == 0 {
		usr.StartOfPeriod = 1
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

func (r *Repository) SetDefaultAccount(ctx context.Context, id string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)
		err := qtx.SetDefaultAccount(ctx, sql.NullString{String: id, Valid: true})
		if err != nil {
			l.Error("setting default account", zap.String("account_id", id), zap.Error(err))
			return err
		}
		return nil
	})
}

func dbUserToUser(dbU *queries.User, dfltCur *queries.Currency, dfltAcc *queries.Account) *models.User {
	u := &models.User{
		ID:            dbU.ID,
		OAuthID:       dbU.OauthID,
		OAuthProvider: models.OAuthProvider(dbU.OauthProvider),
		Email:         dbU.Email,
		VerifiedEmail: dbU.VerifiedEmail,
		FirstName:     dbU.FirstName,
		LastName:      dbU.LastName,
		Picture:       dbU.Picture.String,
		PictureHash:   dbU.PictureHash.String,
		StartOfPeriod: uint8(dbU.StartOfPeriod),
		Timezone:      dbU.Timezone,
		MonthlySalary: dbU.MonthlySalary.Float64,
		DefaultCurrency: models.Currency{
			ID: dbU.DefaultCurrencyID,
		},
		SeededAt:  dbU.SeededAt.Time,
		Onboarded: dbU.OnboardedAt.Valid,
		CreatedAt: dbU.CreatedAt,
	}

	if dfltCur != nil {
		u.DefaultCurrency = *dbCurrencyToCurrency(dfltCur, true)
	}
	u.SalaryCurrency = u.DefaultCurrency.String

	if dfltAcc != nil {
		u.DefaultAccount = dbAccountToAccount(dfltAcc, nil, nil, nil, true)
	}

	return u
}

func dbUserRowToUser(dbU queries.GetUserByOAuthIDRow) *models.User {
	acc := queries.Account{
		ID:                 dbU.AccountID.String,
		Name:               dbU.AccountName.String,
		KindID:             dbU.AccountKindID.String,
		CurrencyID:         dbU.Currency.ID,
		CounterpartyID:     dbU.AccountCounterpartyID,
		CardNumber:         dbU.AccountCardNumber,
		CardLimit:          dbU.AccountCardLimit.Float64,
		NameOnCard:         dbU.AccountNameOnCard,
		Iban:               dbU.AccountIban,
		Note:               dbU.AccountNote,
		IsArchived:         dbU.AccountIsArchived.Bool,
		ExcludeFromBalance: dbU.AccountExcludeFromBalance.Bool,
		ExpirationDate:     dbU.AccountExpirationDate,
		CreatedAt:          dbU.AccountCreatedAt.Time,
	}
	u := dbUserToUser(&dbU.User, &dbU.Currency, &acc)
	if dbU.SalaryCurrencyString.Valid {
		u.SalaryCurrency = dbU.SalaryCurrencyString.String
	}
	return u
}

// GetTimezone is the time zone of the user's sms, the default one if the
// stored one doesn't load.
func (r *Repository) GetTimezone(ctx context.Context) (*time.Location, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	name, err := q.GetTimezone(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.LoadTimezone(models.DefaultTimezone)
	}
	if err != nil {
		return nil, err
	}
	loc, err := models.LoadTimezone(name)
	if err != nil {
		logger.FromCtx(ctx).Warn("invalid stored time zone, using the default")
		return models.LoadTimezone(models.DefaultTimezone)
	}
	return loc, nil
}

func (r *Repository) SetTimezone(ctx context.Context, loc *time.Location) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetTimezone(ctx, loc.String())
}

// SetMonthStart sets the day the user's months start on.
func (r *Repository) SetMonthStart(ctx context.Context, s models.MonthStart) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetStartOfPeriod(ctx, int64(s.Day()))
}

// SetUserName sets the user's first and last name.
func (r *Repository) SetUserName(ctx context.Context, first, last string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetUserName(ctx, queries.SetUserNameParams{FirstName: first, LastName: last})
}

// SetOnboarded records the user finished (or skipped) the onboarding.
func (r *Repository) SetOnboarded(ctx context.Context) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetOnboarded(ctx)
}

// SetUserPicture stores the user's downloaded picture, from the url.
func (r *Repository) SetUserPicture(ctx context.Context, url string, pic *avatar.Picture) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetUserPicture(ctx, queries.SetUserPictureParams{
		Picture:     sql.NullString{String: url, Valid: url != ""},
		PictureData: pic.Data,
		PictureType: sql.NullString{String: pic.ContentType, Valid: true},
		PictureHash: sql.NullString{String: pic.Hash, Valid: true},
	})
}

// GetUserPicture is the user's downloaded picture, sql.ErrNoRows when there's
// none.
func (r *Repository) GetUserPicture(ctx context.Context) (*avatar.Picture, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetUserPicture(ctx)
	if err != nil {
		return nil, err
	}
	return &avatar.Picture{Data: row.PictureData, ContentType: row.PictureType.String, Hash: row.PictureHash.String}, nil
}
