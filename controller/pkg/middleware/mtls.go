package middleware

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"

	"storageos/controller/pkg/ca"
	"storageos/controller/pkg/provisioning"
)

// MTLSMiddleware verifies client certificates from OS nodes
type MTLSMiddleware struct {
	certAuth    *ca.CertificateAuthority
	nodeService *provisioning.NodeService
}

// NewMTLSMiddleware creates a new mTLS middleware
func NewMTLSMiddleware(certAuth *ca.CertificateAuthority, nodeService *provisioning.NodeService) *MTLSMiddleware {
	return &MTLSMiddleware{
		certAuth:    certAuth,
		nodeService: nodeService,
	}
}

// VerifyNodeCertificate middleware verifies that the client has a valid certificate
func (m *MTLSMiddleware) VerifyNodeCertificate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if TLS is enabled
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			log.Printf("mTLS verification failed: no client certificate provided")
			http.Error(w, "Client certificate required", http.StatusUnauthorized)
			return
		}

		// Get client certificate
		clientCert := r.TLS.PeerCertificates[0]

		// Verify certificate with CA
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: clientCert.Raw,
		})

		verifiedCert, err := m.certAuth.VerifyCertificate(certPEM)
		if err != nil {
			log.Printf("mTLS verification failed: %v", err)
			http.Error(w, "Invalid certificate", http.StatusUnauthorized)
			return
		}

		// Get certificate fingerprint
		fingerprint, err := ca.GetCertificateFingerprint(certPEM)
		if err != nil {
			log.Printf("Failed to calculate fingerprint: %v", err)
			http.Error(w, "Certificate verification failed", http.StatusInternalServerError)
			return
		}

		// Check if certificate is revoked
		revoked, err := m.nodeService.IsRevoked(fingerprint)
		if err != nil {
			log.Printf("Failed to check revocation status: %v", err)
			http.Error(w, "Certificate verification failed", http.StatusInternalServerError)
			return
		}

		if revoked {
			log.Printf("mTLS verification failed: certificate revoked (fingerprint: %s)", fingerprint)
			http.Error(w, "Certificate has been revoked", http.StatusUnauthorized)
			return
		}

		// Get node from database
		node, err := m.nodeService.GetNodeByFingerprint(fingerprint)
		if err != nil {
			log.Printf("mTLS verification failed: node not found (fingerprint: %s)", fingerprint)
			http.Error(w, "Node not registered", http.StatusUnauthorized)
			return
		}

		// Check node status
		if node.Status != "active" {
			log.Printf("mTLS verification failed: node status is %s (node: %s)", node.Status, node.ID)
			http.Error(w, fmt.Sprintf("Node is %s", node.Status), http.StatusUnauthorized)
			return
		}

		// Update last seen timestamp
		if err := m.nodeService.UpdateLastSeen(node.ID); err != nil {
			log.Printf("Warning: failed to update last seen for node %s: %v", node.ID, err)
		}

		// Add node info to context
		ctx := context.WithValue(r.Context(), "node_id", node.ID)
		ctx = context.WithValue(ctx, "node_cn", ca.GetCertificateCN(verifiedCert))
		ctx = context.WithValue(ctx, "node_fingerprint", fingerprint)

		log.Printf("mTLS verification successful: node %s (CN: %s)", node.ID, ca.GetCertificateCN(verifiedCert))

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CreateMTLSServerConfig creates a TLS config for mTLS server
func CreateMTLSServerConfig(certAuth *ca.CertificateAuthority) *tls.Config {
	// Create cert pool with CA
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(certAuth.GetCACertPEM())

	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  certPool,
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}
