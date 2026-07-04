package merchantterminaluc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ProvisionInput identifies the merchant to provision a terminal for.
type ProvisionInput struct {
	// MerchantID is the merchant that will own the terminal.
	MerchantID model.MerchantID

	// IdempotencyKey makes retries return the same terminal and API key.
	IdempotencyKey string
}

// ProvisionOutput carries the new terminal and its plaintext API key.
type ProvisionOutput struct {
	// MerchantID is the owning merchant.
	MerchantID model.MerchantID

	// TerminalID is the public terminal identifier.
	TerminalID model.TerminalID

	// CredentialID identifies the issued credential.
	CredentialID model.MerchantTerminalCredentialID

	// CredentialVersion is the version of the issued credential.
	CredentialVersion int64

	// APIKey is the plaintext API key, shown once at issuance (and idempotent
	// replay inside the delivery TTL). Callers must store it; Get never returns it.
	APIKey string

	// CreatedAt is when the terminal's first credential was created.
	CreatedAt time.Time

	// ExpiresAt is when the key can no longer be replayed to an idempotent retry.
	ExpiresAt time.Time

	// Replayed reports that the key came from an earlier identical request.
	Replayed bool
}

// GetInput identifies the merchant whose terminal should be loaded.
type GetInput struct {
	// MerchantID is the merchant taken from the gateway auth subject.
	MerchantID model.MerchantID
}

// GetActiveCredential describes the active API key of a terminal without the
// secret.
type GetActiveCredential struct {
	// CredentialID identifies the active credential.
	CredentialID model.MerchantTerminalCredentialID

	// Version is the active credential version.
	Version int64

	// ActivatedAt is when the credential became active.
	ActivatedAt time.Time
}

// GetTerminal is the merchant-facing view of one terminal.
//
// It never includes plaintext api_key: keys are show-once at Provision/Rotate
// and only a verifier is stored, so GET cannot recover the secret.
type GetTerminal struct {
	// MerchantID is the owning merchant.
	MerchantID model.MerchantID

	// TerminalID is the public terminal identifier.
	TerminalID model.TerminalID

	// Status is the current terminal lifecycle state.
	Status model.MerchantTerminalStatus

	// CreatedAt is when the terminal was created.
	CreatedAt time.Time

	// UpdatedAt is when the terminal was last modified.
	UpdatedAt time.Time

	// ActiveCredential is the current active credential, when one exists.
	ActiveCredential *GetActiveCredential

	// Retiring describes a superseded credential still inside its grace period.
	Retiring *RetiringCredential
}

// GetOutput is the merchant-facing terminal collection for the panel GET.
//
// Merchants currently own at most one terminal, so Items is either empty or a
// single entry. A missing terminal is an empty list, not an error.
type GetOutput struct {
	// Items is the merchant's terminals (zero or one today). Never nil after Get.
	Items []GetTerminal
}

// RotateInput describes a credential rotation for the authenticated merchant's
// terminal.
type RotateInput struct {
	// MerchantID is the merchant taken from the gateway auth subject.
	MerchantID model.MerchantID

	// GracePeriod is how long the previous credential stays accepted. Nil
	// selects Config.DefaultGrace; zero retires the previous credential at once.
	GracePeriod *time.Duration

	// IdempotencyKey makes retries return the same new API key.
	IdempotencyKey string
}

// RetiringCredential describes the credential superseded by a rotation.
type RetiringCredential struct {
	// CredentialID identifies the superseded credential.
	CredentialID model.MerchantTerminalCredentialID

	// Version is the superseded credential version.
	Version int64

	// RetireAt is the deadline until which the superseded credential is accepted.
	RetireAt time.Time
}

// RotateOutput carries the new credential and its plaintext API key.
type RotateOutput struct {
	// MerchantID is the owning merchant.
	MerchantID model.MerchantID

	// TerminalID is the public terminal identifier.
	TerminalID model.TerminalID

	// CredentialID identifies the new credential.
	CredentialID model.MerchantTerminalCredentialID

	// CredentialVersion is the version of the new credential.
	CredentialVersion int64

	// APIKey is the plaintext API key, shown once at rotation (and idempotent
	// replay inside the delivery TTL). Callers must store it; Get never returns it.
	APIKey string

	// ActivatedAt is when the new credential became active.
	ActivatedAt time.Time

	// Retiring describes the superseded credential, when one is still retiring.
	Retiring *RetiringCredential

	// ExpiresAt is when the key can no longer be replayed to an idempotent retry.
	ExpiresAt time.Time

	// Replayed reports that the key came from an earlier identical request.
	Replayed bool
}

// RetireInput identifies a retiring credential to retire immediately.
type RetireInput struct {
	// MerchantID is the merchant taken from the gateway auth subject.
	MerchantID model.MerchantID

	// CredentialID is the credential to retire.
	CredentialID model.MerchantTerminalCredentialID
}

// RetireOutput reports the retired credential.
type RetireOutput struct {
	// CredentialID identifies the retired credential.
	CredentialID model.MerchantTerminalCredentialID

	// Version is the retired credential version.
	Version int64

	// RetiredAt is when the credential was retired.
	RetiredAt time.Time
}
