package crypto

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func TestHardcodedKeyPairMatches(t *testing.T) {
	priv := GetHardcodedPrivateKey()
	pub := GetHardcodedPublicKey()

	derived, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}

	if !bytes.Equal(derived, pub) {
		t.Fatalf("derived public key %x does not match hardcoded %x", derived, pub)
	}
}
