// Package botmessage defines transport-neutral contracts for bidirectional
// communication through bot-based messaging platforms.
//
// The core package intentionally knows nothing about concrete providers such as
// Telegram or Bale, their HTTP APIs, webhook payloads, polling protocols, bot
// tokens, or SDK types. Provider adapters normalize inbound messages before
// handing them to IncomingMessageHandler and implement Sender for outbound
// delivery.
//
// Listening mechanics are deliberately outside this package. A provider adapter
// may receive messages through an HTTP webhook, long polling, or another
// delivery mechanism without changing the core contract.
package botmessage
