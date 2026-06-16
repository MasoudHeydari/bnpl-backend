package postgres

import (
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Queryer is the type set shared by generic PostgreSQL helpers in this package.
// It intentionally accepts both connection and transaction adapters.
type Queryer interface {
	*Conn | *Tx
	repo.Queryer
}
