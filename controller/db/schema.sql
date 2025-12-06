-- ============================================================================
-- CONTROLLER DATABASE SCHEMA
-- ============================================================================
-- This database stores MINIMAL metadata for directory service only:
-- - File/folder IDs, names, sizes, types
-- - Folder structure and counts
-- - Node routing information (which OS node has the file)
--
-- Full metadata (encryption keys, checksums, storage paths, etc.) is stored
-- in the OS storage nodes where the actual files are stored.
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT DEFAULT 'user',
    used_storage INTEGER DEFAULT 0,
    max_storage INTEGER DEFAULT 1073741824,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP,
    status TEXT DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,               -- e.g. node-east-1
    ip_address TEXT,
    hostname TEXT,
    region TEXT,
    status TEXT DEFAULT 'online',
    last_seen TEXT DEFAULT CURRENT_TIMESTAMP,
    total_storage INTEGER,
    used_storage INTEGER,
    capacity_percent REAL
);

CREATE TABLE IF NOT EXISTS storage_nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT,
    ip_address TEXT,
    port INTEGER,
    is_active BOOLEAN DEFAULT 1,
    last_active TEXT,
    quota_mb INTEGER DEFAULT 0,        -- Configured quota in MB
    used_mb INTEGER DEFAULT 0,         -- Current usage in MB
    available_mb INTEGER DEFAULT 0,    -- Available storage in MB
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (url IS NOT NULL AND TRIM(url) <> '')
        OR
        (ip_address IS NOT NULL AND TRIM(ip_address) <> '' AND port IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS peer_servers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    peer_type TEXT NOT NULL CHECK (peer_type IN ('inbound', 'outbound')),
    request_status TEXT DEFAULT 'pending' CHECK (request_status IN ('pending', 'accepted', 'rejected', 'revoked')),
    quota_allocated BIGINT DEFAULT 0,
    quota_used BIGINT DEFAULT 0,
    storage_remaining BIGINT DEFAULT 0,      -- Remaining storage on peer server (updated via health check)
    is_active BOOLEAN DEFAULT 0,
    last_active TEXT,
    request_token TEXT,
    requested_at TEXT DEFAULT CURRENT_TIMESTAMP,
    accepted_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    client_cert TEXT,      -- mTLS client certificate for this server
    client_key TEXT,       -- mTLS client private key for this server
    server_cert TEXT       -- mTLS server certificate for peer server
);

-- Peer file sharing settings
CREATE TABLE IF NOT EXISTS peer_file_sharing_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    enabled BOOLEAN DEFAULT 0,                           -- Master toggle for peer file sharing
    complete_storage_enabled BOOLEAN DEFAULT 0,          -- Store all shards in peer server (not just copies)
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Peer file sharing outbound configuration (servers we send to)
CREATE TABLE IF NOT EXISTS peer_sharing_outbound (
    id TEXT PRIMARY KEY,
    peer_server_id TEXT NOT NULL REFERENCES peer_servers(id),
    enabled BOOLEAN DEFAULT 1,
    priority INTEGER DEFAULT 0,                          -- Lower = higher priority
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(peer_server_id)
);

-- Peer file sharing inbound configuration (servers that send to us)
CREATE TABLE IF NOT EXISTS peer_sharing_inbound (
    id TEXT PRIMARY KEY,
    peer_server_id TEXT NOT NULL REFERENCES peer_servers(id),
    enabled BOOLEAN DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(peer_server_id)
);

CREATE TABLE IF NOT EXISTS folders (
    id TEXT PRIMARY KEY,
    owner_id TEXT REFERENCES users(id),
    parent_id TEXT REFERENCES folders(id),
    name TEXT NOT NULL,
    is_archived BOOLEAN DEFAULT 0,
    archive_file_id TEXT,
    total_files INTEGER DEFAULT 0,
    total_folders INTEGER DEFAULT 0,
    total_size BIGINT DEFAULT 0,
    deleted_at TEXT,                  -- NULL if not in trash, timestamp if moved to trash
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    modified_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Minimal controller metadata (directory service only)
CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    owner_id TEXT REFERENCES users(id),
    folder_id TEXT REFERENCES folders(id),
    filename TEXT NOT NULL,
    extension TEXT,
    original_extension TEXT,
    mime_type TEXT,
    size BIGINT DEFAULT 0,
    original_size BIGINT DEFAULT 0,
    checksum TEXT,
    storage_path TEXT,
    encrypted_key BLOB,
    ephemeral_pub_key BLOB,
    node_id TEXT,               -- which OS node stores it (for routing)
    source_server_id TEXT,      -- NULL if local, peer server ID if received from peer
    version INTEGER DEFAULT 1,
    status TEXT DEFAULT 'active',
    is_indexed BOOLEAN DEFAULT 0,
    parent_archive_id TEXT,
    relative_path TEXT,
    rs_data_shards INTEGER DEFAULT 0,    -- Reed-Solomon data shard count
    rs_parity_shards INTEGER DEFAULT 0,  -- Reed-Solomon parity shard count
    total_shards INTEGER DEFAULT 0,      -- Total shard count (data + parity)
    is_erasure_coded BOOLEAN DEFAULT 0,  -- Whether file uses RS encoding
    deleted_at TEXT,                     -- NULL if not in trash, timestamp if moved to trash
    uploaded_at TEXT DEFAULT CURRENT_TIMESTAMP,
    modified_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_server_id) REFERENCES peer_servers(id)
);

-- File shards for Reed-Solomon erasure coding
CREATE TABLE IF NOT EXISTS file_shards (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    shard_index INTEGER NOT NULL,
    shard_type TEXT NOT NULL CHECK (shard_type IN ('data', 'parity')),
    node_id TEXT NOT NULL,
    peer_server_id TEXT,                -- NULL if stored locally, peer server ID if stored on peer
    shard_size INTEGER NOT NULL,
    checksum TEXT,
    storage_path TEXT,
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'degraded', 'lost', 'recovering')),
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id),
    FOREIGN KEY (node_id) REFERENCES storage_nodes(id),
    FOREIGN KEY (peer_server_id) REFERENCES peer_servers(id),
    UNIQUE(file_id, shard_index)
);

CREATE TABLE IF NOT EXISTS file_shares (
    id TEXT PRIMARY KEY,
    file_id TEXT REFERENCES files(id),
    owner_id TEXT REFERENCES users(id),
    shared_with TEXT REFERENCES users(id),
    access TEXT DEFAULT 'read',    -- read / write / edit
    expires_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS recent (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    target_id TEXT NOT NULL,       -- can refer to file_id or folder_id
    target_type TEXT NOT NULL,     -- 'file' or 'folder'
    accessed_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS favorite (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    target_id TEXT NOT NULL,       -- can refer to file_id or folder_id
    target_type TEXT NOT NULL,     -- 'file' or 'folder'
    added_at TEXT DEFAULT (datetime('now'))
);

-- Pending deletions for offline nodes (trash cleanup queue)
CREATE TABLE IF NOT EXISTS pending_deletions (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    shard_id TEXT,                 -- If deleting a specific shard
    node_id TEXT,                  -- OS node that needs cleanup
    peer_server_id TEXT,           -- Peer server that needs cleanup (if stored there)
    storage_path TEXT,
    target_type TEXT NOT NULL CHECK (target_type IN ('file', 'shard', 'folder')),
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'failed')),
    retry_count INTEGER DEFAULT 0,
    last_retry_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    error_message TEXT,
    FOREIGN KEY (node_id) REFERENCES storage_nodes(id),
    FOREIGN KEY (peer_server_id) REFERENCES peer_servers(id)
);

-- Indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_files_owner_id ON files(owner_id);
CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);
CREATE INDEX IF NOT EXISTS idx_files_filename ON files(filename);
CREATE INDEX IF NOT EXISTS idx_folders_owner_id ON folders(owner_id);
CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON folders(parent_id);
CREATE INDEX IF NOT EXISTS idx_recent_user_id ON recent(user_id);
CREATE INDEX IF NOT EXISTS idx_favorite_user_id ON favorite(user_id);

-- Indexes for file shards
CREATE INDEX IF NOT EXISTS idx_file_shards_file_id ON file_shards(file_id);
CREATE INDEX IF NOT EXISTS idx_file_shards_node_id ON file_shards(node_id);
CREATE INDEX IF NOT EXISTS idx_file_shards_status ON file_shards(status);
CREATE INDEX IF NOT EXISTS idx_file_shards_peer_server_id ON file_shards(peer_server_id);

-- Indexes for files with peer source
CREATE INDEX IF NOT EXISTS idx_files_source_server_id ON files(source_server_id);

-- NOTE: Trash indexes (idx_files_deleted_at, idx_folders_deleted_at) and pending_deletions indexes
-- are created in migrations.go to handle existing databases that don't have these columns yet
