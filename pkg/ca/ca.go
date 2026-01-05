// Package ca provides certificate authority management for MITM proxy.
package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CA represents a certificate authority for MITM proxying.
type CA struct {
	Certificate *x509.Certificate
	PrivateKey  *ecdsa.PrivateKey
	certPEM     []byte
	keyPEM      []byte
}

// Config holds CA configuration options.
type Config struct {
	// Organization name for the CA certificate
	Organization string
	// CommonName for the CA certificate
	CommonName string
	// ValidFor is how long the CA is valid (default: 10 years)
	ValidFor time.Duration
}

// DefaultConfig returns default CA configuration.
func DefaultConfig() *Config {
	return &Config{
		Organization: "OmniProxy",
		CommonName:   "OmniProxy Root CA",
		ValidFor:     10 * 365 * 24 * time.Hour, // 10 years
	}
}

// New creates a new CA with the given configuration.
func New(cfg *Config) (*CA, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Generate ECDSA P-256 private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(cfg.ValidFor)

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cfg.Organization},
			CommonName:   cfg.CommonName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CA{
		Certificate: cert,
		PrivateKey:  privateKey,
		certPEM:     certPEM,
		keyPEM:      keyPEM,
	}, nil
}

// Load loads an existing CA from PEM files.
func Load(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	return LoadFromPEM(certPEM, keyPEM)
}

// LoadFromPEM loads a CA from PEM-encoded data.
func LoadFromPEM(certPEM, keyPEM []byte) (*CA, error) {
	// Parse certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	var privateKey *ecdsa.PrivateKey

	// Try ECDSA first, then PKCS8
	privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		// Try PKCS8
		key, err2 := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not ECDSA")
		}
	}

	return &CA{
		Certificate: cert,
		PrivateKey:  privateKey,
		certPEM:     certPEM,
		keyPEM:      keyPEM,
	}, nil
}

// Save saves the CA certificate and private key to files.
func (ca *CA) Save(certPath, keyPath string) error {
	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("failed to create certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Write certificate (readable by all)
	if err := os.WriteFile(certPath, ca.certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write private key (owner only)
	if err := os.WriteFile(keyPath, ca.keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}

// CertPEM returns the CA certificate in PEM format.
func (ca *CA) CertPEM() []byte {
	return ca.certPEM
}

// KeyPEM returns the CA private key in PEM format.
func (ca *CA) KeyPEM() []byte {
	return ca.keyPEM
}

// TLSCertificate returns the CA as a tls.Certificate.
func (ca *CA) TLSCertificate() (tls.Certificate, error) {
	return tls.X509KeyPair(ca.certPEM, ca.keyPEM)
}

// GenerateCert generates a certificate for the given domain, signed by this CA.
func (ca *CA) GenerateCert(domain string) (certPEM, keyPEM []byte, err error) {
	// Generate new private key for this certificate
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:   time.Now().Add(-1 * time.Hour), // 1 hour before to handle clock skew
		NotAfter:    time.Now().AddDate(1, 0, 0),    // 1 year validity
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Certificate, &privateKey.PublicKey, ca.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// DefaultCADir returns the default directory for storing CA files.
func DefaultCADir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".omniproxy"
	}
	return filepath.Join(home, ".omniproxy", "ca")
}

// DefaultCertPath returns the default path for the CA certificate.
func DefaultCertPath() string {
	return filepath.Join(DefaultCADir(), "omniproxy-ca.crt")
}

// DefaultKeyPath returns the default path for the CA private key.
func DefaultKeyPath() string {
	return filepath.Join(DefaultCADir(), "omniproxy-ca.key")
}

// LoadOrCreate loads an existing CA or creates a new one if it doesn't exist.
func LoadOrCreate(certPath, keyPath string, cfg *Config) (*CA, error) {
	// Try to load existing CA
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return Load(certPath, keyPath)
		}
	}

	// Create new CA
	ca, err := New(cfg)
	if err != nil {
		return nil, err
	}

	// Save it
	if err := ca.Save(certPath, keyPath); err != nil {
		return nil, err
	}

	return ca, nil
}
