package storeclient

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

// SignedManifestPrefix is the canonical string an ed25519 signature
// covers: the package sha256 hex digest, prefixed and versioned. It
// must match the Store's lib/signing.js manifestString().
const SignedManifestPrefix = "timekeeper-package/v1:"

// CanonicalString returns the exact bytes the Store signs for a package.
func CanonicalString(sha256Hex string) string {
	return SignedManifestPrefix + sha256Hex
}

// ParsePublicKey decodes a SPKI PEM ed25519 public key published by
// the Store. Only a public key may be parsed here; a private key is
// rejected explicitly so a misconfigured Store cannot trick the client
// into trusting material it should not have.
func ParsePublicKey(pemText string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("store public key is not PEM encoded")
	}
	if block.Type == "PRIVATE KEY" || block.Type == "ENCRYPTED PRIVATE KEY" {
		return nil, fmt.Errorf("refusing a private key from the Store: only public verification keys may be used")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse store public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("store public key is not ed25519")
	}
	return key, nil
}

// SHA256File returns the hex sha256 of a file.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// VerifyPackage proves two things before the extension runtime ever
// sees a package:
//
//   1. the bytes match the checksum the Store signed, and
//   2. the signature was made by the Store's signing key.
//
// A modified package, a truncated download and a package signed by a
// different key all fail here.
func VerifyPackage(path, wantSHA256, signatureBase64, publicKeyPEM string) error {
	actual, err := SHA256File(path)
	if err != nil {
		return fmt.Errorf("hash package: %w", err)
	}
	if wantSHA256 != "" && actual != wantSHA256 {
		return fmt.Errorf("package checksum mismatch: expected %s, got %s", wantSHA256, actual)
	}
	if signatureBase64 == "" {
		return fmt.Errorf("package has no signature; refusing to install")
	}
	key, err := ParsePublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("decode package signature: %w", err)
	}
	if len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("package signature has the wrong length")
	}
	if !ed25519.Verify(key, []byte(CanonicalString(actual)), raw) {
		return fmt.Errorf("package signature is invalid: this package did not come from the official Store")
	}
	return nil
}
