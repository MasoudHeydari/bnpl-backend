package repo

import "context"

type TxHandler func(context.Context, Tx) error

type Conn interface {
	Queryer

	Tx(ctx context.Context, handler TxHandler) error

	// IsConn method prevents a non-Conn object to mistakenly implement
	// the Conn interface.
	IsConn()
}
