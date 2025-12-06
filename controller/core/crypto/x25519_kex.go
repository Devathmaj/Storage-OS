package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// X25519 key-exchange helpers.

// Hardcoded keypair for testing purposes.
// In production, these should be securely generated and stored.
var (
	// HardcodedPrivateKey is a test private key (32 bytes)
	// Generated from: crypto/rand
	hardcodedPrivHex = "a8f5c7e2b9d4f1a6c3e8b5d2f9a4c7e1b8d5f2a9c6e3b0d7f4a1c8e5b2d9f6a3"

	// HardcodedPublicKey matches the derived public key for hardcodedPrivHex
	hardcodedPubHex = "b6e008f84aa0827b191c35a6bd8460aa60973748377fccbbcb00bc107c7d8a3d"
)

// GetHardcodedPrivateKey returns the hardcoded private key for testing
func GetHardcodedPrivateKey() []byte {
	priv, err := hex.DecodeString(hardcodedPrivHex)
	if err != nil {
		panic("invalid hardcoded private key: " + err.Error())
	}
	return priv
}

// GetHardcodedPublicKey returns the hardcoded public key for testing
func GetHardcodedPublicKey() []byte {
	pub, err := hex.DecodeString(hardcodedPubHex)
	if err != nil {
		panic("invalid hardcoded public key: " + err.Error())
	}
	return pub
}

// GenerateX25519KeyPair generates a new X25519 key pair.
// Returns private and public keys as 32-byte slices.
func GenerateX25519KeyPair() (priv, pub []byte, err error) {
	priv = make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, priv); err != nil {
		return nil, nil, err
	}
	// Clamp private scalar per RFC7748
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err = curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

// DeriveSharedSecret computes the X25519 shared secret from a private key and a peer public key.
// Both keys must be 32 bytes. Returns 32-byte raw shared secret.
func DeriveSharedSecret(priv, peerPub []byte) ([]byte, error) {
	if len(priv) != 32 || len(peerPub) != 32 {
		return nil, errors.New("keys must be 32 bytes")
	}
	return curve25519.X25519(priv, peerPub)
}

// DeriveAES256KeyFromShared derives a 32-byte AES-256 key from the raw X25519 shared secret
// using HKDF-SHA256. The optional info can be used to context-separate different uses.
func DeriveAES256KeyFromShared(shared []byte, info []byte) ([]byte, error) {
	hk := hkdf.New(sha256.New, shared, nil, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(hk, key); err != nil {
		return nil, err
	}
	return key, nil
}

// EncryptFileWithX25519 derives a symmetric key using X25519 (senderPriv and receiverPub),
// then encrypts the src file to dst using AES-256-GCM (via EncryptFile).
// The info parameter is optional context for HKDF; use nil or []byte("file-encryption-v1").
func EncryptFileWithX25519(srcPath, dstPath string, senderPriv, receiverPub, info []byte) error {
	shared, err := DeriveSharedSecret(senderPriv, receiverPub)
	if err != nil {
		return err
	}
	key, err := DeriveAES256KeyFromShared(shared, info)
	if err != nil {
		return err
	}
	return EncryptFile(srcPath, dstPath, key)
}

// DecryptFileWithX25519 derives a symmetric key using X25519 (receiverPriv and senderPub),
// then decrypts the src file to dst using AES-256-GCM (via DecryptFile).
func DecryptFileWithX25519(srcPath, dstPath string, receiverPriv, senderPub, info []byte) error {
	shared, err := DeriveSharedSecret(receiverPriv, senderPub)
	if err != nil {
		return err
	}
	key, err := DeriveAES256KeyFromShared(shared, info)
	if err != nil {
		return err
	}
	return DecryptFile(srcPath, dstPath, key)
}
