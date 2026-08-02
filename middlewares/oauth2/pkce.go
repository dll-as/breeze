package oauth2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// randomString returns n bytes of cryptographically secure randomness encoded
// as URL-safe base64 without padding. Used for secrets, state, nonce and PKCE
// verifiers. It panics only if the system CSPRNG fails, which is unrecoverable.
func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("oauth2: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// pkcePair holds a generated PKCE verifier and its S256 challenge.
type pkcePair struct {
	Verifier  string
	Challenge string
	Method    string // always "S256"
}

// newPKCE generates a fresh PKCE verifier/challenge pair using the S256 method
// (RFC 7636). The verifier is 32 random bytes (43 base64url chars) — within the
// 43..128 length the RFC requires. The challenge is BASE64URL(SHA256(verifier)).
func newPKCE() pkcePair {
	verifier := randomString(32)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return pkcePair{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    "S256",
	}
}

// verifyPKCE reports whether challenge equals BASE64URL(SHA256(verifier)). Used
// in tests and defensive checks; the provider performs the authoritative check.
func verifyPKCE(verifier, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}
