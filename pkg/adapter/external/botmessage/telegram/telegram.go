// Package telegram implements bot messaging through the Telegram Bot API.
//
// Outbound sendMessage calls use github.com/go-telegram/bot so the adapter
// follows the current Telegram Bot API model. Inbound updates use explicit
// getUpdates long polling so the adapter controls when an update offset is
// advanced relative to application processing.
package telegram
