package repo

// Role identifies a persistence-layer role. See doc/architecture/database-roles.md.
type Role string

const (
	// AdminRole provisions roles and transfers managed ownership; never used by Runtime.
	AdminRole Role = "admin"
	// OwnerRole owns managed objects and has NOLOGIN; it has no application credential.
	OwnerRole Role = "bnpl_owner"
	// MigratorRole logs in without inherited ownership and explicitly assumes OwnerRole.
	MigratorRole Role = "bnpl_migrator"
	// NormalRole has only reviewed business DML, never ownership, DELETE or TRUNCATE.
	NormalRole Role = "bnpl"
)
