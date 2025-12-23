package postgres

import (
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type Queryer interface {
	*Conn | *Tx
	repo.Queryer
}
