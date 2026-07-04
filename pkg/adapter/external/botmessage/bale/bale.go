// Package bale implements bot messaging through the Bale Bot API.
//
// Bale follows Telegram Bot API concepts but is not assumed to be wire-identical
// to the current Telegram API. This adapter therefore uses Bale's documented
// HTTP schema directly, including reply_to_message_id for replies.
package bale
