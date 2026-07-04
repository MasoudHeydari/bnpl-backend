package secretbox

// Box encrypts and decrypts small secrets.
type Box interface {
	// Encrypt seals plaintext with the active key and returns the encoded
	// ciphertext together with the id of the key that produced it.
	Encrypt(plaintext string) (ciphertext string, keyID string, err error)

	// Decrypt opens ciphertext using the key identified by keyID. Retired keys
	// remain available for decryption until they are removed from configuration.
	Decrypt(ciphertext string, keyID string) (plaintext string, err error)
}
