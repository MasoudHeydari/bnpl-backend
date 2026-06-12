package repo

// Tx abstracts a database transaction that can execute queries.
// Transaction values are scoped to a single Conn.Tx callback and should not be
// retained after the callback returns.
type Tx interface {
	Queryer

	IsTx()
}
