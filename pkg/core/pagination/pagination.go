// Package pagination belongs to the core layer and must not depend on HTTP, Gin,
// PostgreSQL, or any transport/infrastructure package. Repositories must map
// logical sort fields to safe storage columns; never concatenate SortField
// directly into SQL.
package pagination
