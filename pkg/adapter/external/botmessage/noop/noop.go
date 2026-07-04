// Package noop provides a bot messaging adapter that performs no external I/O.
//
// It satisfies the core botmessage contracts without calling Telegram, Bale, or
// any other provider. Use it when bot messaging is disabled or in tests.
package noop
