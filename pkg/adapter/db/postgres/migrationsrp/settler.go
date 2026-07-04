package migrationsrp

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Settler implements the core schema initializer contract.
var _ repo.SchemaInitializer = (*Settler)(nil)

// Settler initializes a freshly created schema by executing embedded SQL
// assets inside a caller-managed transaction.
type Settler struct {
	tx repo.Tx // destination database transaction
}

// NewSettler constructs a schema initializer bound to tx.
// The transaction must remain open for the lifetime of the initializer call.
func NewSettler(tx repo.Tx) *Settler {
	return &Settler{tx: tx}
}

//go:embed 000_schema.sql
var schemaDDLStatements string

// baselineSchemaSHA256 identifies the canonical pre-release schema. The file
// becomes immutable when the first Production version ships; subsequent schema
// evolution then belongs exclusively in versioned incremental migrations.
// Updating this digest is intentionally review-visible.
const baselineSchemaSHA256 = "d00dbeb5695a17577ffce38e6dd2faf00c1819df70e847be264723ddc8f26dd0"

func validateBaselineSchema() error {
	digest := sha256.Sum256([]byte(schemaDDLStatements))
	actual := hex.EncodeToString(digest[:])
	if actual != baselineSchemaSHA256 {
		return fmt.Errorf(
			"baseline schema checksum changed: expected=%s actual=%s; inspect the baseline and review the corresponding checksum update",
			baselineSchemaSHA256,
			actual,
		)
	}

	return nil
}
