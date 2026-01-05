package ca

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	ca, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	if ca.Certificate == nil {
		t.Error("expected certificate")
	}

	if ca.PrivateKey == nil {
		t.Error("expected private key")
	}

	if !ca.Certificate.IsCA {
		t.Error("expected IsCA to be true")
	}

	if ca.Certificate.Subject.CommonName != "OmniProxy Root CA" {
		t.Errorf("expected CommonName 'OmniProxy Root CA', got %s", ca.Certificate.Subject.CommonName)
	}
}

func TestNewWithConfig(t *testing.T) {
	cfg := &Config{
		Organization: "TestOrg",
		CommonName:   "Test CA",
		ValidFor:     24 * time.Hour,
	}

	ca, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	if ca.Certificate.Subject.Organization[0] != "TestOrg" {
		t.Errorf("expected Organization 'TestOrg', got %v", ca.Certificate.Subject.Organization)
	}

	if ca.Certificate.Subject.CommonName != "Test CA" {
		t.Errorf("expected CommonName 'Test CA', got %s", ca.Certificate.Subject.CommonName)
	}

	// Check validity period
	expectedExpiry := time.Now().Add(24 * time.Hour)
	if ca.Certificate.NotAfter.Before(expectedExpiry.Add(-1*time.Minute)) ||
		ca.Certificate.NotAfter.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("unexpected expiry time: %v", ca.Certificate.NotAfter)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "omniproxy-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	// Create and save CA
	ca1, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	if err := ca1.Save(certPath, keyPath); err != nil {
		t.Fatalf("failed to save CA: %v", err)
	}

	// Check file permissions
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("failed to stat key file: %v", err)
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Errorf("expected key file permissions 0600, got %o", keyInfo.Mode().Perm())
	}

	// Load CA
	ca2, err := Load(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load CA: %v", err)
	}

	// Compare
	if ca2.Certificate.Subject.CommonName != ca1.Certificate.Subject.CommonName {
		t.Errorf("CommonName mismatch: %s vs %s",
			ca2.Certificate.Subject.CommonName, ca1.Certificate.Subject.CommonName)
	}

	if ca2.Certificate.SerialNumber.Cmp(ca1.Certificate.SerialNumber) != 0 {
		t.Error("SerialNumber mismatch")
	}
}

func TestGenerateCert(t *testing.T) {
	ca, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	domain := "api.example.com"
	certPEM, keyPEM, err := ca.GenerateCert(domain)
	if err != nil {
		t.Fatalf("failed to generate cert: %v", err)
	}

	// Parse the generated certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to parse generated cert: %v", err)
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse x509 cert: %v", err)
	}

	// Verify domain
	if cert.Subject.CommonName != domain {
		t.Errorf("expected CommonName %s, got %s", domain, cert.Subject.CommonName)
	}

	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != domain {
		t.Errorf("expected DNSNames [%s], got %v", domain, cert.DNSNames)
	}

	// Verify signed by CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   domain,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("certificate verification failed: %v", err)
	}
}

func TestTLSCertificate(t *testing.T) {
	ca, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	tlsCert, err := ca.TLSCertificate()
	if err != nil {
		t.Fatalf("failed to get TLS certificate: %v", err)
	}

	if len(tlsCert.Certificate) == 0 {
		t.Error("expected certificate chain")
	}
}

func TestCertPEM(t *testing.T) {
	ca, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	certPEM := ca.CertPEM()
	if len(certPEM) == 0 {
		t.Error("expected cert PEM data")
	}

	// Should start with PEM header
	if string(certPEM[:27]) != "-----BEGIN CERTIFICATE-----" {
		t.Error("expected PEM header")
	}
}

func TestLoadOrCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "omniproxy-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	// First call should create
	ca1, err := LoadOrCreate(certPath, keyPath, nil)
	if err != nil {
		t.Fatalf("failed to load or create CA: %v", err)
	}

	// Second call should load existing
	ca2, err := LoadOrCreate(certPath, keyPath, nil)
	if err != nil {
		t.Fatalf("failed to load or create CA: %v", err)
	}

	// Should be same CA
	if ca2.Certificate.SerialNumber.Cmp(ca1.Certificate.SerialNumber) != 0 {
		t.Error("expected same CA to be loaded")
	}
}

func TestLoadFromPEM(t *testing.T) {
	ca1, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	ca2, err := LoadFromPEM(ca1.CertPEM(), ca1.KeyPEM())
	if err != nil {
		t.Fatalf("failed to load from PEM: %v", err)
	}

	if ca2.Certificate.Subject.CommonName != ca1.Certificate.Subject.CommonName {
		t.Error("CommonName mismatch")
	}
}
