// Package secretbox provides an AES-256-GCM implementation of the core secret
// box.
//
// Ciphertexts have the form "sb1.<base64url(nonce || sealed)>". The key id is
// not embedded; callers persist it beside the ciphertext. The key id is bound
// into the GCM additional data, so a ciphertext cannot be decrypted under a
// different key id label, and a fresh random nonce is used for every call.
//
// Multiple keys may be configured so ciphertexts written under a retired key
// keep decrypting during a rotation window while new writes use the active key.
package secretbox
