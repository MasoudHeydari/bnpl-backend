// Package querylog provides development-only PostgreSQL query logging for pgx.
//
// When enabled, each Query / QueryRow / Exec is logged before execution so local
// and staging environments can inspect SQL without a DB proxy. Production must
// keep query logging disabled. Logging never changes what pgx executes;
// argument rendering is display-only.
package querylog
