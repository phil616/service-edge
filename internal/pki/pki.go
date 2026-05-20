// Package pki implements the control-plane certificate authority: loading and
// validating the configured CA at startup, and signing short-lived leaf certs
// for frps servers and frpc clients.
//
// frp (v0.50+) enables TLS by default and supports mutual verification: frps
// presents server.crt, frpc presents client.crt, and both trust the same CA via
// transport.tls.trustedCaFile. Go 1.15+ ignores CommonName for hostname
// verification, so leaf certs carry SANs. frp's TLS verification (when
// trustedCaFile is set) checks the chain against the CA, not the hostname, so a
// CN of frps-<uuid>/frpc-<uuid> plus a DNS SAN is sufficient.
package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

const (
	leafValidity   = 90 * 24 * time.Hour
	RenewThreshold = 30 * 24 * time.Hour
	keyBits        = 2048
)

// CA holds the loaded, validated certificate authority.
type CA struct {
	cert    *x509.Certificate
	key     *rsa.PrivateKey
	certPEM []byte
}

// IssuedCert is a freshly signed leaf cert + key in PEM form.
type IssuedCert struct {
	CertPEM string
	KeyPEM  string
}

// LoadCA reads and validates the CA cert/key from disk. It returns an error
// (callers panic on it) if the files are missing, mismatched, expired or the
// cert is not a CA.
func LoadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read ca cert: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read ca key: %w", err)
	}

	cert, err := parseCert(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parse ca cert: %w", err)
	}
	key, err := parseKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse ca key: %w", err)
	}

	// Validity window.
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return nil, fmt.Errorf("ca cert not yet valid (notBefore=%s)", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return nil, fmt.Errorf("ca cert expired (notAfter=%s)", cert.NotAfter)
	}

	// Must be a CA (root or valid intermediate).
	if !cert.IsCA {
		return nil, fmt.Errorf("certificate is not a CA (BasicConstraints.IsCA=false)")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, fmt.Errorf("ca cert lacks KeyUsageCertSign")
	}

	// Public key of cert must match private key.
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ca cert public key is not RSA")
	}
	if pub.N.Cmp(key.N) != 0 || pub.E != key.E {
		return nil, fmt.Errorf("ca cert and key do not match")
	}

	return &CA{cert: cert, key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})}, nil
}

// CertPEM returns the CA certificate in PEM form (for trustedCaFile distribution).
func (ca *CA) CertPEM() string { return string(ca.certPEM) }

// IssueServerCert signs an frps server certificate (CN frps-<uuid>).
func (ca *CA) IssueServerCert(uuid string, dnsNames []string) (*IssuedCert, error) {
	return ca.issue("frps-"+uuid, dnsNames, x509.ExtKeyUsageServerAuth)
}

// IssueClientCert signs an frpc client certificate (CN frpc-<uuid>).
func (ca *CA) IssueClientCert(uuid string) (*IssuedCert, error) {
	return ca.issue("frpc-"+uuid, []string{"frpc-" + uuid}, x509.ExtKeyUsageClientAuth)
}

func (ca *CA) issue(cn string, dnsNames []string, eku x509.ExtKeyUsage) (*IssuedCert, error) {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	if len(dnsNames) == 0 {
		dnsNames = []string{cn}
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{eku},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign leaf cert: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8(key)})
	return &IssuedCert{CertPEM: string(certPEM), KeyPEM: string(keyPEM)}, nil
}

// NeedsRenewal reports whether the given PEM cert is within RenewThreshold of
// expiry (or unparseable, in which case it should be reissued).
func NeedsRenewal(certPEM string) bool {
	cert, err := parseCert([]byte(certPEM))
	if err != nil {
		return true
	}
	return time.Until(cert.NotAfter) <= RenewThreshold
}

func parseCert(p []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(p)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseKey(p []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(p)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("ca key is not RSA")
	}
	return key, nil
}

func pkcs8(key *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		// MarshalPKCS8PrivateKey only fails for unsupported key types.
		return x509.MarshalPKCS1PrivateKey(key)
	}
	return b
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	return serial, nil
}
