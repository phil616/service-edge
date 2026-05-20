package pki

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeCA(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	certPath = filepath.Join(dir, "ca.crt")
	keyPath = filepath.Join(dir, "ca.key")
	certPEM, keyPEM, err := GenerateCAToString()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	if err := os.WriteFile(certPath, []byte(certPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestLoadCAValidatesAndSigns(t *testing.T) {
	certPath, keyPath := writeCA(t)
	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}

	leaf, err := ca.IssueServerCert("abc-123", []string{"frps-abc-123", "203.0.113.5"})
	if err != nil {
		t.Fatalf("IssueServerCert: %v", err)
	}

	// The issued leaf must chain to the CA.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(ca.CertPEM())) {
		t.Fatal("failed to add CA to pool")
	}
	block, _ := pem.Decode([]byte(leaf.CertPEM))
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if _, err := parsed.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatalf("leaf does not chain to CA: %v", err)
	}
	if parsed.Subject.CommonName != "frps-abc-123" {
		t.Errorf("CN = %q, want frps-abc-123", parsed.Subject.CommonName)
	}
}

func TestLoadCARejectsMismatchedKey(t *testing.T) {
	certPath, _ := writeCA(t)
	_, otherKey := writeCA(t)
	if _, err := LoadCA(certPath, otherKey); err == nil {
		t.Fatal("expected error for mismatched cert/key, got nil")
	}
}

func TestLoadCARejectsNonCACert(t *testing.T) {
	// A leaf cert is not a CA and must be rejected.
	certPath, keyPath := writeCA(t)
	ca, _ := LoadCA(certPath, keyPath)
	leaf, _ := ca.IssueClientCert("xyz")

	dir := t.TempDir()
	lc := filepath.Join(dir, "leaf.crt")
	lk := filepath.Join(dir, "leaf.key")
	os.WriteFile(lc, []byte(leaf.CertPEM), 0o600)
	os.WriteFile(lk, []byte(leaf.KeyPEM), 0o600)
	if _, err := LoadCA(lc, lk); err == nil {
		t.Fatal("expected rejection of non-CA cert, got nil")
	}
}
