-- ============================================================================
-- OS NODE DATABASE SCHEMA
-- ============================================================================
-- This database stores FULL metadata for files stored on this OS node:
-- - Physical storage paths (/data/files/{owner_id}/{file_id})
-- - Encryption keys, checksums, compression info
-- - File versions and status
--
-- This is separate from the controller database which only stores minimal
-- routing information. The OS node owns the source of truth for file metadata.
-- ============================================================================

-- Files stored on this OS node
CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,                  -- UUID
    owner_id TEXT NOT NULL,              -- User who owns the file
    folder_id TEXT,                      -- Optional folder (from controller)
    filename TEXT NOT NULL,              -- Original filename
    extension TEXT,                      -- File extension (.jpg, .pdf, etc.)
    original_extension TEXT,             -- Extension before processing
    mime_type TEXT,                      -- Content type
    size INTEGER NOT NULL,               -- File size in bytes
    checksum TEXT NOT NULL,              -- SHA-256 checksum
    storage_path TEXT NOT NULL,          -- Physical path: /data/files/{owner_id}/{file_id}
    node_id TEXT NOT NULL,               -- This node's identifier
    version INTEGER DEFAULT 1,           -- File version (for updates)
    status TEXT DEFAULT 'active',        -- active / deleted / quarantined
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    accessed_at TIMESTAMP
);

-- Encryption metadata (if using encrypted storage)
CREATE TABLE IF NOT EXISTS encryption_metadata (
    file_id TEXT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
    algorithm TEXT NOT NULL,             -- AES-256-GCM, ChaCha20-Poly1305, etc.
    encrypted_key TEXT,                  -- Symmetric key encrypted with X25519
    ephemeral_public_key TEXT,           -- Ephemeral public key for key decryption
    key_id TEXT,                         -- Reference to key management system
    iv TEXT,                             -- Initialization vector
    auth_tag TEXT,                       -- Authentication tag for AEAD
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Chunk metadata for large files
CREATE TABLE IF NOT EXISTS file_chunks (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    chunk_size INTEGER NOT NULL,
    chunk_checksum TEXT NOT NULL,
    storage_path TEXT NOT NULL,          -- /data/chunks/{file_id}/{chunk_index}
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(file_id, chunk_index)
);

-- Compression metadata
CREATE TABLE IF NOT EXISTS compression_metadata (
    file_id TEXT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
    algorithm TEXT NOT NULL,             -- gzip, zstd, lz4, etc.
    original_size INTEGER NOT NULL,
    compressed_size INTEGER NOT NULL,
    compression_ratio REAL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Storage quota tracking per user on this node
CREATE TABLE IF NOT EXISTS user_quotas (
    owner_id TEXT PRIMARY KEY,
    used_storage INTEGER DEFAULT 0,
    max_storage INTEGER,
    file_count INTEGER DEFAULT 0,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_files_owner_id ON files(owner_id);
CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);
CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
CREATE INDEX IF NOT EXISTS idx_files_checksum ON files(checksum);
CREATE INDEX IF NOT EXISTS idx_files_uploaded_at ON files(uploaded_at);
CREATE INDEX IF NOT EXISTS idx_file_chunks_file_id ON file_chunks(file_id);
CREATE INDEX IF NOT EXISTS idx_user_quotas_owner_id ON user_quotas(owner_id);

-- Triggers to update quota on file operations
CREATE TRIGGER IF NOT EXISTS update_quota_on_insert
AFTER INSERT ON files
FOR EACH ROW
BEGIN
    INSERT INTO user_quotas (owner_id, used_storage, file_count)
    VALUES (NEW.owner_id, NEW.size, 1)
    ON CONFLICT(owner_id) DO UPDATE SET
        used_storage = used_storage + NEW.size,
        file_count = file_count + 1,
        last_updated = CURRENT_TIMESTAMP;
END;

CREATE TRIGGER IF NOT EXISTS update_quota_on_delete
AFTER UPDATE OF status ON files
FOR EACH ROW
WHEN NEW.status = 'deleted' AND OLD.status != 'deleted'
BEGIN
    UPDATE user_quotas
    SET used_storage = used_storage - OLD.size,
        file_count = file_count - 1,
        last_updated = CURRENT_TIMESTAMP
    WHERE owner_id = OLD.owner_id;
END;
