-- ============================================================================
-- OS NODE PROVISIONING & MTLS AUTHENTICATION SCHEMA
-- ============================================================================
-- This schema implements OTP-based enrollment with manual approval and mTLS
-- ============================================================================

-- One-time passwords for OS node enrollment
CREATE TABLE IF NOT EXISTS enrollment_otps (
    otp TEXT PRIMARY KEY,                    -- 6-digit numeric OTP
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,           -- Valid for 5 minutes
    used_at TIMESTAMP,                       -- NULL if unused
    used_by_node_id TEXT,                    -- Node that used this OTP
    created_by_user_id TEXT,                 -- Admin who generated it
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'used', 'expired', 'revoked'))
);

-- Pending enrollment requests awaiting admin approval
CREATE TABLE IF NOT EXISTS enrollment_requests (
    id TEXT PRIMARY KEY,
    otp TEXT NOT NULL,
    csr TEXT NOT NULL,                       -- Base64 encoded CSR
    system_info TEXT NOT NULL,               -- JSON with hostname, cpu, os_version, network_id
    requested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reviewed_at TIMESTAMP,
    reviewed_by_user_id TEXT,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    rejection_reason TEXT
);

-- Enrolled OS nodes with certificates
CREATE TABLE IF NOT EXISTS os_nodes (
    id TEXT PRIMARY KEY,                     -- Unique node ID (e.g., node-44)
    certificate TEXT NOT NULL,               -- Base64 encoded certificate
    certificate_fingerprint TEXT UNIQUE NOT NULL,  -- SHA256 fingerprint for quick lookup
    certificate_cn TEXT NOT NULL,            -- Common Name from certificate
    private_key_generated_by_node BOOLEAN DEFAULT 1,  -- Node generates its own private key
    enrollment_request_id TEXT REFERENCES enrollment_requests(id),
    system_info TEXT NOT NULL,               -- JSON with node details
    enrolled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    certificate_expires_at TIMESTAMP NOT NULL,
    last_seen TIMESTAMP,
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'revoked', 'expired', 'removed')),
    revoked_at TIMESTAMP,
    revoked_by_user_id TEXT,
    revocation_reason TEXT
);

-- Certificate revocation list
CREATE TABLE IF NOT EXISTS certificate_revocations (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES os_nodes(id),
    certificate_fingerprint TEXT NOT NULL,
    certificate_cn TEXT NOT NULL,
    revoked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    revoked_by_user_id TEXT,
    reason TEXT NOT NULL
);

-- Certificate renewal history
CREATE TABLE IF NOT EXISTS certificate_renewals (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES os_nodes(id),
    old_certificate_fingerprint TEXT NOT NULL,
    new_certificate_fingerprint TEXT NOT NULL,
    renewed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    new_expires_at TIMESTAMP NOT NULL
);

-- WebSocket connections tracking
CREATE TABLE IF NOT EXISTS node_connections (
    node_id TEXT PRIMARY KEY REFERENCES os_nodes(id),
    connected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat TIMESTAMP,
    connection_status TEXT DEFAULT 'connected' CHECK (connection_status IN ('connected', 'disconnected')),
    ip_address TEXT,
    user_agent TEXT
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_enrollment_otps_expires ON enrollment_otps(expires_at);
CREATE INDEX IF NOT EXISTS idx_enrollment_otps_status ON enrollment_otps(status);
CREATE INDEX IF NOT EXISTS idx_enrollment_requests_status ON enrollment_requests(status);
CREATE INDEX IF NOT EXISTS idx_enrollment_requests_requested_at ON enrollment_requests(requested_at);
CREATE INDEX IF NOT EXISTS idx_os_nodes_status ON os_nodes(status);
CREATE INDEX IF NOT EXISTS idx_os_nodes_fingerprint ON os_nodes(certificate_fingerprint);
CREATE INDEX IF NOT EXISTS idx_os_nodes_cn ON os_nodes(certificate_cn);
CREATE INDEX IF NOT EXISTS idx_certificate_revocations_fingerprint ON certificate_revocations(certificate_fingerprint);
CREATE INDEX IF NOT EXISTS idx_certificate_revocations_cn ON certificate_revocations(certificate_cn);
