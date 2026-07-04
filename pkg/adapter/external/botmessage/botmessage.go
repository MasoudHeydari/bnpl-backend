// Package botmessage contains provider adapters and runtime coordination for
// bot-based messaging integrations.
//
// The package owns provider selection and routing only. Provider payloads are
// normalized to the core botmessage contracts before they reach application
// handlers, and business behavior remains in core use cases.
package botmessage
