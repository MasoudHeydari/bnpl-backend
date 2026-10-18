package header

const (
	// AuthSubjectID carries the authenticated external subject UUID issued by
	// Identity Service (x-auth-subject-id). It maps to customer_profiles.external_customer_id
	// and is never the internal customer_profiles.id primary key.
	AuthSubjectID = "x-auth-subject-id"

	// AuthTenantID carries the authenticated tenant UUID.
	AuthTenantID = "x-auth-tenant-id"

	// AuthContextID carries the authenticated business-context UUID.
	AuthContextID = "x-auth-context-id"

	// AuthPermissions carries comma-separated gateway permissions.
	AuthPermissions = "x-auth-permissions"
)
