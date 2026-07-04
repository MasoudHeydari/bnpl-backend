// Package iran implements Iranian mobile phone validation and canonicalization.
//
// Accepted raw forms are 09xxxxxxxxx and +989xxxxxxxxx. Both canonicalize to
// +989xxxxxxxxx. Whitespace, punctuation, and non-ASCII digits are rejected.
package iran
