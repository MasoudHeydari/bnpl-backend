package policy

// Ref identifies a specific versioned policy implementation within a policy
// family. Ref values are comparable and are used as registry keys.
type Ref struct {
	// ID is the stable policy family identifier.
	ID string
	// Version is the hard-coded policy version label within the family.
	Version string
}

// String returns a human-readable "id/version" representation, useful for
// error messages and audit logging.
func (r Ref) String() string {
	return r.ID + "/" + r.Version
}

// IsZero reports whether r carries no identity. Registries reject IsZero
// refs at registration time.
func (r Ref) IsZero() bool {
	return r.ID == "" && r.Version == ""
}
