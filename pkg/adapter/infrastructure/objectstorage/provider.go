package objectstorage

// Provider identifies which object-storage adapter to construct.
type Provider string

const (
	// NoopProvider disables durable object persistence. It is useful for tests or
	// deployments that have not enabled archival yet.
	NoopProvider Provider = "noop"

	// FilesystemProvider stores objects below a local root directory. It is
	// intended for local development and integration tests, not multi-node
	// production archival.
	FilesystemProvider Provider = "filesystem"
)
