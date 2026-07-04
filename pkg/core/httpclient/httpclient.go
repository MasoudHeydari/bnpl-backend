// Package httpclient defines the application-owned contract for outbound
// HTTP/HTTPS execution.
//
// The contract lives in Core so infrastructure adapters depend inward on an
// application port rather than defining the port themselves. Business use
// cases should still depend on domain-specific external-service interfaces
// not on this generic HTTP abstraction.
package httpclient
