package repo

// Tx abstracts a database transaction that can execute queries.
// Transaction values are scoped to a single Conn.Tx callback and should not be
// retained after the callback returns.
type Tx interface {
	// Queryer provides statement execution and row queries inside the transaction.
	Queryer

	// IsTx method prevents a non-Tx object from mistakenly implementing the Tx
	// interface.
	IsTx()
}
