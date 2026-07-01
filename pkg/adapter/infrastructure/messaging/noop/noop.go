// Package noop provides no-op messaging adapters.
//
// These adapters satisfy the core messaging contracts without connecting to any
// external broker. They are useful for local development, tests, and
// environments where messaging is intentionally disabled.
//
// Important: noop messaging does not persist, deliver, retry, or acknowledge
// messages. It must not be used for workflows that require reliable event
// delivery.
package noop
