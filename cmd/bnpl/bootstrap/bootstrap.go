// Package bootstrap composes configuration, infrastructure, repositories,
// use cases, background tasks, and HTTP routes for the BNPL application.
//
// It belongs to the adapter layer and orchestrates startup without defining
// domain rules. CLI commands invoke Boot, Migrate, and the build-profile
// schema init entry points (InitDev or InitProd) to run the corresponding
// startup workflows.
package bootstrap
