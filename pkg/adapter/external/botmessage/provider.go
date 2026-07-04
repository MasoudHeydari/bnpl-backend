package botmessage

// IntegrationID identifies one configured bot transport.
//
// These identifiers intentionally live in the adapter layer. Core treats the
// integration identifier carried by a Destination as an opaque routing value.
type IntegrationID string

const (
	// NoopIntegrationID disables external bot I/O while preserving wiring.
	NoopIntegrationID IntegrationID = "noop"

	// TelegramIntegrationID routes messages through Telegram Bot API.
	TelegramIntegrationID IntegrationID = "telegram"

	// BaleIntegrationID routes messages through Bale Bot API.
	BaleIntegrationID IntegrationID = "bale"
)
