// Package secretbox defines the core port for envelope-encrypting small secrets
// (for example merchant webhook signing secrets) before they are persisted.
//
// Ciphertexts are versioned, self-contained strings that are safe for a text
// column. The key id returned by Encrypt must be stored next to the ciphertext
// so Decrypt can select the right key after key rotation.
package secretbox
