// Package inprocess provides an in-process task runtime.
//
// The in-process runtime executes registered jobs inside the same application
// process. It supports both scheduled execution and on-demand dispatch.
//
// Supported scheduled triggers:
//
//   - manual: registered but not executed automatically
//   - cron: executed by robfig/cron
//   - interval: executed by goroutine and time.Ticker
//
// Dispatch resolves commands by Handler and executes the matching Job
// asynchronously. This provider is suitable for local development, simple
// deployments, and early-stage systems before introducing Temporal or
// distributed workers.
package inprocess
