package repo

// Role identifies a persistence-layer role used by the application.
type Role string

const (
	// AdminRole is used for schema bootstrap tasks that require elevated
	// privileges such as schema recreation and role management.
	AdminRole Role = "admin"
	// NormalRole is the application role used for ordinary runtime queries and
	// migration execution after bootstrap.
	NormalRole Role = "bnpl"
)
