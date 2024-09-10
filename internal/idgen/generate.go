package idgen

import (
	"math/rand"
	"time"

	"github.com/oklog/ulid"
)

func New() string {
	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	return ulid.MustNew(ulid.Now(), entropy).String()
}
