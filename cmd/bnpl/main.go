// Package main wires the BNPL CLI entrypoint.
// It belongs to the adapter layer and delegates process startup
// to the Cobra command package without containing business logic.
package main

import "github.com/parstis/bnpl-backend/cmd/bnpl/command"

// main executes the root CLI command and exits through Cobra's error handling.
func main() {
	command.Execute()
}
