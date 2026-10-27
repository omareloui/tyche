// Package dbtypes holds custom column types used by sqlc generated code.
package dbtypes

import (
	"database/sql/driver"
	"fmt"
)

// JSON is a raw JSON column that scans from both TEXT and BLOB values, as
// SQLite returns TEXT for literal defaults like '[]'.
type JSON []byte

func (j *JSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = JSON(v)
	default:
		return fmt.Errorf("dbtypes.JSON: unsupported scan type %T", src)
	}
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
}
