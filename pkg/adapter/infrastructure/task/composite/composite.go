// Package composite provides a task runtime that fans work across in-process
// and Temporal backends.
//
// Jobs are routed by configuration so some handlers run locally while others
// execute as Temporal workflows.
package composite
