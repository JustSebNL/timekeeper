package storeclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func testKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	return pub, priv, pemText
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.tkp")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return path
}

func sha256Of(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}

func TestVerifyPackageAcceptsAProperlySignedPackage(t *testing.T) {
	_, priv, pemText := testKeyPair(t)
	path := writeTempFile(t, "extension bytes")
	sum := sha256Of("extension bytes")
	sig := ed25519.Sign(priv, []byte(CanonicalString(sum)))

	if err := VerifyPackage(path, sum, base64.StdEncoding.EncodeToString(sig), pemText); err != nil {
		t.Fatalf("expected a valid package, got: %v", err)
	}
}

func TestVerifyPackageRejectsATamperedPackage(t *testing.T) {
	_, priv, pemText := testKeyPair(t)
	path := writeTempFile(t, "extension bytes")
	sum := sha256Of("extension bytes")
	sig := ed25519.Sign(priv, []byte(CanonicalString(sum)))

	// Change the bytes after signing.
	if err := os.WriteFile(path, []byte("extension bytes!"), 0o644); err != nil {
		t.Fatalf("rewrite temp: %v", err)
	}
	err := VerifyPackage(path, sum, base64.StdEncoding.EncodeToString(sig), pemText)
	if err == nil {
		t.Fatal("expected a tampered package to be rejected")
	}
}

func TestVerifyPackageRejectsAForeignKey(t *testing.T) {
	_, otherPriv, _ := testKeyPair(t)
	_, _, pemText := testKeyPair(t)
	path := writeTempFile(t, "extension bytes")
	sum := sha256Of("extension bytes")
	sig := ed25519.Sign(otherPriv, []byte(CanonicalString(sum)))

	if err := VerifyPackage(path, sum, base64.StdEncoding.EncodeToString(sig), pemText); err == nil {
		t.Fatal("expected a package signed by another key to be rejected")
	}
}

func TestVerifyPackageRequiresASignature(t *testing.T) {
	_, _, pemText := testKeyPair(t)
	path := writeTempFile(t, "extension bytes")
	sum := sha256Of("extension bytes")

	if err := VerifyPackage(path, sum, "", pemText); err == nil {
		t.Fatal("expected an unsigned package to be rejected")
	}
}

func TestParsePublicKeyRefusesAPrivateKey(t *testing.T) {
	_, priv, _ := testKeyPair(t)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	if _, err := ParsePublicKey(pemText); err == nil {
		t.Fatal("the client must never accept a private key from the Store")
	}
}

func TestReadAndVerifyPackageReturnsStableBytesForExtraction(t *testing.T) {
	_, priv, pemText := testKeyPair(t)
	entries := validEntries()
	entries = append(entries, entry{name: "./checksums.json", content: checksumsFor(entries)})
	archive := buildTkp(t, entries)
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive fixture: %v", err)
	}
	sum := sha256.Sum256(data)
	sha := fmt.Sprintf("%x", sum[:])
	sig := ed25519.Sign(priv, []byte(CanonicalString(sha)))

	verified, err := ReadAndVerifyPackage(archive, sha, base64.StdEncoding.EncodeToString(sig), pemText)
	if err != nil {
		t.Fatalf("read and verify: %v", err)
	}
	if err := os.WriteFile(archive, []byte("replacement after verification"), 0o644); err != nil {
		t.Fatalf("replace archive: %v", err)
	}

	manifest, err := ExtractPackageBytes(verified, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("extract verified bytes: %v", err)
	}
	if manifest.ID != "extension.test" || manifest.Version != "1.0.0" {
		t.Fatalf("unexpected manifest from verified bytes: %+v", manifest)
	}
}

func TestCanonicalStringMatchesTheStoreFormat(t *testing.T) {
	got := CanonicalString("abc123")
	want := "timekeeper-package/v1:abc123"
	if got != want {
		t.Fatalf("canonical string mismatch: got %q want %q", got, want)
	}
}

func TestNewInstallationIDIsStableAndWellFormed(t *testing.T) {
	first := NewInstallationID()
	second := NewInstallationID()
	if first == second {
		t.Fatal("two installations must not share an identity")
	}
	if len(first) != len("TK-DEVICE-")+16 {
		t.Fatalf("unexpected installation id: %q", first)
	}
	if first[:len("TK-DEVICE-")] != "TK-DEVICE-" {
		t.Fatalf("unexpected installation id prefix: %q", first)
	}
}
