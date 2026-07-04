// Package objectstorage defines provider-neutral blob storage contracts.
//
// The core contract is intentionally small. Business use cases must not depend
// on S3/MinIO/filesystem details, and destructive purge operations are kept out
// of the normal runtime interface so audit/forensic data cannot be deleted by
// ordinary application code.
package objectstorage
