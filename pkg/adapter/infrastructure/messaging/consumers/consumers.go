// Package consumers registers and runs Kafka message consumer handlers.
//
// It wires topic handlers into a shared registry with logging and panic
// reporting so bootstrap can start all consumers uniformly.
package consumers
