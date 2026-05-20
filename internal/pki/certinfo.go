package pki

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

// CertInfo is a JSON-friendly summary of an X.509 certificate, used to surface
// CA and leaf certificate details in the UI without exposing key material.
type CertInfo struct {
	Subject            string    `json:"subject"`
	Issuer             string    `json:"issuer"`
	SerialNumber       string    `json:"serial_number"`
	NotBefore          time.Time `json:"not_before"`
	NotAfter           time.Time `json:"not_after"`
	IsCA               bool      `json:"is_ca"`
	DNSNames           []string  `json:"dns_names,omitempty"`
	SignatureAlgorithm string    `json:"signature_algorithm"`
	PublicKeyAlgorithm string    `json:"public_key_algorithm"`
	KeyBits            int       `json:"key_bits,omitempty"`
	FingerprintSHA256  string    `json:"fingerprint_sha256"`
	DaysRemaining      int       `json:"days_remaining"`
	Expired            bool      `json:"expired"`
}

// Info returns a summary of the loaded CA certificate.
func (ca *CA) Info() *CertInfo {
	return certToInfo(ca.cert)
}

// ParseCertInfo parses a PEM-encoded certificate into a CertInfo summary.
func ParseCertInfo(certPEM string) (*CertInfo, error) {
	cert, err := parseCert([]byte(certPEM))
	if err != nil {
		return nil, fmt.Errorf("parse cert: %w", err)
	}
	return certToInfo(cert), nil
}

func certToInfo(cert *x509.Certificate) *CertInfo {
	sum := sha256.Sum256(cert.Raw)
	fp := strings.ToUpper(hex.EncodeToString(sum[:]))
	// Group the hex fingerprint into colon-separated byte pairs for readability.
	var parts []string
	for i := 0; i+2 <= len(fp); i += 2 {
		parts = append(parts, fp[i:i+2])
	}

	days := int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))

	return &CertInfo{
		Subject:            cert.Subject.String(),
		Issuer:             cert.Issuer.String(),
		SerialNumber:       cert.SerialNumber.String(),
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		IsCA:               cert.IsCA,
		DNSNames:           cert.DNSNames,
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		PublicKeyAlgorithm: cert.PublicKeyAlgorithm.String(),
		KeyBits:            publicKeyBits(cert),
		FingerprintSHA256:  strings.Join(parts, ":"),
		DaysRemaining:      days,
		Expired:            time.Now().After(cert.NotAfter),
	}
}

func publicKeyBits(cert *x509.Certificate) int {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return pub.N.BitLen()
	case *ecdsa.PublicKey:
		return pub.Curve.Params().BitSize
	default:
		return 0
	}
}
