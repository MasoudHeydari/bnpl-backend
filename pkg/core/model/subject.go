package model

import (
	"database/sql/driver"
	"fmt"

	"github.com/google/uuid"
)

// Authenticated request identity types.
//
// SubjectID, TenantID, and ContextID wrap the trusted UUIDs attached to a
// request by the API gateway / Identity Service. They are named types so the
// compiler rejects accidental swaps with entity primary keys such as
// CustomerID or ContractID.
//
// Mapping rules:
//   - For customer contracts/installments list APIs, SubjectID equals
//     ExternalSubjectID (x-auth-subject-id → customer_profiles.external_subject_id).
//   - Customer credit APIs resolve Customer Reference Service customer_id via
//     mobile (x-auth-mobile-number) into ExternalCustomerID
//     (customer_profiles.external_customer_id); that value is not SubjectID.
//   - For merchant APIs, SubjectID equals ExternalMerchantID
//     (x-auth-subject-id → merchant_profiles.external_merchant_id).
//   - SubjectID is never customer_profiles.id (CustomerID) or
//     merchant_profiles.id (MerchantProfileID).
//
// Convert at the use-case boundary with AsExternalSubjectID,
// AsExternalCustomerID, or AsExternalMerchantID according to the caller's role.
// Do not cross-cast these types into entity primary keys.

// SubjectID is the Identity Service subject UUID from x-auth-subject-id.
//
// Its meaning is role-polymorphic: customer callers carry an ExternalSubjectID
// (and separately a Customer Reference ExternalCustomerID); merchant callers
// carry an ExternalMerchantID. Use AsExternalSubjectID / AsExternalCustomerID /
// AsExternalMerchantID at the authenticated entrypoint; never compare SubjectID
// directly with internal customer_id / merchant_profile_id columns.
type SubjectID uuid.UUID

// ParseSubjectID parses a SubjectID from its canonical UUID string form.
func ParseSubjectID(value string) (SubjectID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return SubjectID{}, fmt.Errorf("parse subject id: %w", err)
	}

	return SubjectID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id SubjectID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id SubjectID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id SubjectID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// AsExternalSubjectID interprets the authenticated subject as the Identity
// Service subject stored on customer_profiles.external_subject_id.
//
// Call this on customer-authenticated paths that key the BNPL profile by
// x-auth-subject-id (for example contracts and installments list). Resolve with
// GetByExternalSubjectID, then use profile.ID (CustomerID) for internal foreign
// keys. Do not treat this as ExternalCustomerID or CustomerID.
func (id SubjectID) AsExternalSubjectID() ExternalSubjectID {
	return ExternalSubjectID(id)
}

// AsExternalCustomerID interprets the authenticated subject as an external
// Customer Reference Service customer_id (customer_profiles.external_customer_id).
//
// Prefer AsExternalSubjectID for Identity-subject keyed customer APIs. Keep this
// helper for paths that still resolve profiles by ExternalCustomerID. Use
// profile.ID (CustomerID) for internal foreign keys after lookup.
func (id SubjectID) AsExternalCustomerID() ExternalCustomerID {
	return ExternalCustomerID(id)
}

// AsExternalMerchantID interprets the authenticated subject as an external merchant
// identity (merchant_profiles.external_merchant_id).
//
// Call this only on merchant-authenticated paths. Resolve the BNPL merchant
// profile with GetByMerchantID, then use profile.ID (MerchantProfileID) for
// internal foreign keys.
func (id SubjectID) AsExternalMerchantID() ExternalMerchantID {
	return ExternalMerchantID(id)
}

// MarshalText implements encoding.TextMarshaler.
func (id SubjectID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *SubjectID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = SubjectID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *SubjectID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = SubjectID(u)

	return nil
}

// Value implements driver.Valuer.
func (id SubjectID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// TenantID identifies the authenticated tenant scope for a request
// (x-auth-tenant-id and related gateway metadata).
//
// It is distinct from Financial Core string tenant identifiers stored on
// payment attempts. Contract cancellation snapshots may persist this value as
// CancellationTenantID.
type TenantID uuid.UUID

// ParseTenantID parses a TenantID from its canonical UUID string form.
func ParseTenantID(value string) (TenantID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return TenantID{}, fmt.Errorf("parse tenant id: %w", err)
	}

	return TenantID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id TenantID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id TenantID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id TenantID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText implements encoding.TextMarshaler.
func (id TenantID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *TenantID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = TenantID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *TenantID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = TenantID(u)

	return nil
}

// Value implements driver.Valuer.
func (id TenantID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// ContextID identifies the authenticated security / business context for a
// request (x-auth-context-id and related gateway metadata).
//
// It is distinct from Financial Core string context identifiers stored on
// payment attempts. Contract cancellation snapshots may persist this value as
// CancellationContextID.
type ContextID uuid.UUID

// ParseContextID parses a ContextID from its canonical UUID string form.
func ParseContextID(value string) (ContextID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return ContextID{}, fmt.Errorf("parse context id: %w", err)
	}

	return ContextID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id ContextID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id ContextID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id ContextID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText implements encoding.TextMarshaler.
func (id ContextID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *ContextID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = ContextID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *ContextID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = ContextID(u)

	return nil
}

// Value implements driver.Valuer.
func (id ContextID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }
