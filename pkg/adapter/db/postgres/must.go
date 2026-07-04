package postgres

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// MustConn returns c as *Conn.
//
// A wrong concrete type is a bootstrap or wiring bug and panics.
func MustConn(c repo.Conn) *Conn {
	cc, ok := c.(*Conn)
	if !ok {
		panic(fmt.Sprintf("postgres.MustConn: expected *Conn, got %T", c))
	}

	return cc
}

// MustTx returns tx as *Tx.
//
// A wrong concrete type is a bootstrap or wiring bug and panics.
func MustTx(tx repo.Tx) *Tx {
	tt, ok := tx.(*Tx)
	if !ok {
		panic(fmt.Sprintf("postgres.MustTx: expected *Tx, got %T", tx))
	}

	return tt
}

// MustRows returns rows as the PostgreSQL Rows adapter type.
//
// A wrong concrete type is an adapter bug and panics.
func MustRows(rows repo.Rows) Rows {
	r, ok := rows.(Rows)
	if !ok {
		panic(fmt.Sprintf("postgres.MustRows: expected postgres.Rows, got %T", rows))
	}

	return r
}
