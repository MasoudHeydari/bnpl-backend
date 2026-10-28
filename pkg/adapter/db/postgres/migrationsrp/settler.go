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

// baselineSchemaSHA256 freezes the pre-production baseline schema. Once this
// baseline ships, schema evolution must happen through assets/db/migrations so
// a fresh database and an upgraded database traverse the same incremental
// changes. Editing 000_schema.sql requires an explicit code change here and is
// therefore intentionally review-visible.
const baselineSchemaSHA256 = "3d6a6bea1a70b181771376cb676633b7f693619a233c8c8e86d0b29a1ff7043f"

func validateBaselineSchema() error {
	digest := sha256.Sum256([]byte(schemaDDLStatements))
	actual := hex.EncodeToString(digest[:])
	if actual != baselineSchemaSHA256 {
		return fmt.Errorf(
			"baseline schema checksum changed: expected=%s actual=%s; add an incremental migration instead of editing 000_schema.sql",
			baselineSchemaSHA256,
			actual,
		)
	}

	return nil
}
