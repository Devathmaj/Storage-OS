package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	// CertificateValidity is the default certificate lifetime (90 days)
	CertificateValidity = 90 * 24 * time.Hour
	
	// CAKeyFile is the CA private key filename
	CAKeyFile = "ca-key.pem"
	
	// CACertFile is the CA certificate filename
	CACertFile = "ca-cert.pem"
)

// CertificateAuthority manages certificate issuance and revocation
type CertificateAuthority struct {
	privateKey  *ecdsa.PrivateKey
	certificate *x509.Certificate
	certPEM     []byte
	certDir     string
}

// NewCA creates or loads a Certificate Authority
func NewCA(certDir string) (*CertificateAuthority, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("create cert directory: %w", err)
	}

	ca := &CertificateAuthority{
		certDir: certDir,
	}

	keyPath := filepath.Join(certDir, CAKeyFile)
	certPath := filepath.Join(certDir, CACertFile)

	// Check if CA already exists
	if _, err := os.Stat(keyPath); err == nil {
		// Load existing CA
		if err := ca.loadCA(keyPath, certPath); err != nil {
			return nil, fmt.Errorf("load existing CA: %w", err)
		}
		return ca, nil
	}

	// Generate new CA
	if err := ca.generateCA(); err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// Save CA to disk
	if err := ca.saveCA(keyPath, certPath); err != nil {
		return nil, fmt.Errorf("save CA: %w", err)
	}

	return ca, nil
}

// generateCA creates a new ECDSA CA certificate
func (ca *CertificateAuthority) generateCA() error {
	// Generate ECDSA private key (P-256 curve)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate private key: %w", err)
	}
	ca.privateKey = privateKey

	// Create CA certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"StorageOS"},
			CommonName:   "StorageOS Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Self-sign the CA certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	ca.certificate = cert

	// Encode to PEM
	ca.certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return nil
}

// saveCA saves the CA private key and certificate to disk
func (ca *CertificateAuthority) saveCA(keyPath, certPath string) error {
	// Marshal private key to PKCS8
	keyBytes, err := x509.MarshalPKCS8PrivateKey(ca.privateKey)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	if err := os.WriteFile(certPath, ca.certPEM, 0644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	return nil
}

// loadCA loads an existing CA from disk
func (ca *CertificateAuthority) loadCA(keyPath, certPath string) error {
	// Load private key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("not an ECDSA private key")
	}
	ca.privateKey = ecdsaKey

	// Load certificate
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read certificate: %w", err)
	}
	ca.certPEM = certPEM

	block, _ = pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode certificate PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	ca.certificate = cert

	return nil
}

// SignCSR signs a certificate signing request and returns the certificate
func (ca *CertificateAuthority) SignCSR(csrPEM []byte, nodeID string) (certPEM []byte, expiresAt time.Time, err error) {
	// Decode PEM
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, time.Time{}, fmt.Errorf("failed to decode CSR PEM")
	}

	// Parse CSR
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse CSR: %w", err)
	}

	// Verify CSR signature
	if err := csr.CheckSignature(); err != nil {
		return nil, time.Time{}, fmt.Errorf("invalid CSR signature: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("generate serial number: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(CertificateValidity)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"StorageOS"},
			CommonName:   nodeID,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, ca.certificate, csr.PublicKey, ca.privateKey)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM, notAfter, nil
}

// SignCertificateFromPublicKey creates a certificate directly from a public key (for browser certificates)
func (ca *CertificateAuthority) SignCertificateFromPublicKey(publicKeyBytes []byte, nodeID string) (certPEM []byte, expiresAt time.Time, err error) {
	// Parse the public key
	publicKey, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse public key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("generate serial number: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(CertificateValidity)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"StorageOS"},
			CommonName:   nodeID,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, ca.certificate, publicKey, ca.privateKey)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM, notAfter, nil
}

// GetCACertBase64 returns the CA certificate in base64 format
func (ca *CertificateAuthority) GetCACertBase64() string {
	return base64.StdEncoding.EncodeToString(ca.certPEM)
}

// VerifyCertificate verifies a client certificate against the CA
func (ca *CertificateAuthority) VerifyCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Create cert pool with CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.certificate)

	// Verify certificate
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		return nil, fmt.Errorf("verify certificate: %w", err)
	}

	return cert, nil
}

// GetCertificateFingerprint calculates SHA256 fingerprint of a certificate
func GetCertificateFingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}

	hash := sha256.Sum256(block.Bytes)
	return fmt.Sprintf("%x", hash), nil
}

// GetCertificate returns the CA certificate
func (ca *CertificateAuthority) GetCertificate() *x509.Certificate {
	return ca.certificate
}

// GenerateServerCertificate generates a server certificate for the controller
func (ca *CertificateAuthority) GenerateServerCertificate(dnsNames []string, ipAddresses []string) (certPEM []byte, keyPEM []byte, err error) {
	// Generate ECDSA private key (P-256 curve)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %w", err)
	}

	// Convert IP addresses
	var ips []net.IP
	for _, ip := range ipAddresses {
		ips = append(ips, net.ParseIP(ip))
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year for server cert

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"StorageOS"},
			CommonName:   "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	// Sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, ca.certificate, &privateKey.PublicKey, ca.privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Marshal private key to PKCS8
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}
