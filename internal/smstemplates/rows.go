package smstemplates

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/omareloui/tyche/internal/dbtypes"
	"github.com/omareloui/tyche/internal/models"
)

// Row is a template as stored, the same in the shared and the users'
// databases (but for the shared seed key), so the generated types of both
// convert to it.
type Row struct {
	ID        string
	Name      string
	Bank      string
	Senders   dbtypes.JSON
	Action    string
	TxKind    sql.NullInt64
	Body      string
	Fixed     dbtypes.JSON
	Sample    string
	Priority  int64
	IsEnabled bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Params are the stored values of a template, see Row.
type Params struct {
	ID        string
	Name      string
	Bank      string
	Senders   dbtypes.JSON
	Action    string
	TxKind    sql.NullInt64
	Body      string
	Fixed     dbtypes.JSON
	Sample    string
	Priority  int64
	IsEnabled bool
}

func (r Row) Model(scope models.SMSTemplateScope) models.SMSTemplate {
	t := models.SMSTemplate{
		ID:        r.ID,
		Scope:     scope,
		Name:      r.Name,
		Bank:      r.Bank,
		Action:    models.SMSTemplateAction(r.Action),
		TxKind:    models.TxKind(r.TxKind.Int64),
		Body:      r.Body,
		Sample:    r.Sample,
		Priority:  int(r.Priority),
		IsEnabled: r.IsEnabled,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	_ = json.Unmarshal(r.Senders, &t.Senders)
	_ = json.Unmarshal(r.Fixed, &t.Fixed)
	return t
}

func ParamsOf(t *models.SMSTemplate) Params {
	senders, _ := json.Marshal(nonNil(t.Senders))
	fixed := []byte("{}")
	if len(t.Fixed) > 0 {
		fixed, _ = json.Marshal(t.Fixed)
	}
	return Params{
		ID:        t.ID,
		Name:      t.Name,
		Bank:      t.Bank,
		Senders:   senders,
		Action:    string(t.Action),
		TxKind:    sql.NullInt64{Int64: int64(t.TxKind), Valid: t.TxKind != models.NilTxKind},
		Body:      t.Body,
		Fixed:     fixed,
		Sample:    t.Sample,
		Priority:  int64(t.Priority),
		IsEnabled: t.IsEnabled,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
