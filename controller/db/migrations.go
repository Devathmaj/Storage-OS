package db

import (
	"database/sql"
	"log"
)

// RunMigrations applies any pending database migrations
func RunMigrations(db *sql.DB) error {
	// Check if last_active column exists in storage_nodes
	var columnExists bool
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('storage_nodes') WHERE name = 'last_active'`)
	if err := row.Scan(&columnExists); err != nil {
		log.Printf("Warning: could not check for last_active column: %v", err)
	}

	if !columnExists {
		log.Println("Adding last_active column to storage_nodes table...")
		_, err := db.Exec(`ALTER TABLE storage_nodes ADD COLUMN last_active TEXT`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: added last_active column")
	}

	// Check if peer_servers table exists
	var tableExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='peer_servers'`)
	if err := row.Scan(&tableExists); err != nil {
		log.Printf("Warning: could not check for peer_servers table: %v", err)
	}

	if !tableExists {
		log.Println("Creating peer_servers table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS peer_servers (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				peer_type TEXT NOT NULL CHECK (peer_type IN ('inbound', 'outbound')),
				request_status TEXT DEFAULT 'pending' CHECK (request_status IN ('pending', 'accepted', 'rejected', 'revoked')),
				quota_allocated BIGINT DEFAULT 0,
				quota_used BIGINT DEFAULT 0,
				is_active BOOLEAN DEFAULT 0,
				last_active TEXT,
				request_token TEXT,
				requested_at TEXT DEFAULT CURRENT_TIMESTAMP,
				accepted_at TEXT,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
				client_cert TEXT,
				client_key TEXT,
				server_cert TEXT
			)
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created peer_servers table")
	} else {
		// Check if certificate columns exist and add them if missing
		var clientCertExists bool
		row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('peer_servers') WHERE name = 'client_cert'`)
		if err := row.Scan(&clientCertExists); err != nil {
			log.Printf("Warning: could not check for client_cert column: %v", err)
		}

		if !clientCertExists {
			log.Println("Adding certificate columns to peer_servers table...")
			_, err := db.Exec(`ALTER TABLE peer_servers ADD COLUMN client_cert TEXT`)
			if err != nil {
				log.Printf("Warning: could not add client_cert column: %v", err)
			}
			_, err = db.Exec(`ALTER TABLE peer_servers ADD COLUMN client_key TEXT`)
			if err != nil {
				log.Printf("Warning: could not add client_key column: %v", err)
			}
			_, err = db.Exec(`ALTER TABLE peer_servers ADD COLUMN server_cert TEXT`)
			if err != nil {
				log.Printf("Warning: could not add server_cert column: %v", err)
			}
			log.Println("✓ Migration complete: added certificate columns to peer_servers table")
		}
	}

	// Check if provisioning tables exist (for OS node mTLS enrollment)
	var provisioningTablesExist bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='enrollment_otps'`)
	if err := row.Scan(&provisioningTablesExist); err != nil {
		log.Printf("Warning: could not check for enrollment_otps table: %v", err)
	}

	if !provisioningTablesExist {
		log.Println("Creating OS node provisioning tables...")
		_, err := db.Exec(`
			-- One-time passwords for OS node enrollment
			CREATE TABLE IF NOT EXISTS enrollment_otps (
				otp TEXT PRIMARY KEY,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				expires_at TIMESTAMP NOT NULL,
				used_at TIMESTAMP,
				used_by_node_id TEXT,
				created_by_user_id TEXT,
				status TEXT DEFAULT 'active' CHECK (status IN ('active', 'used', 'expired', 'revoked'))
			);

			-- Pending enrollment requests awaiting admin approval
			CREATE TABLE IF NOT EXISTS enrollment_requests (
				id TEXT PRIMARY KEY,
				otp TEXT NOT NULL,
				csr TEXT NOT NULL,
				system_info TEXT NOT NULL,
				requested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				reviewed_at TIMESTAMP,
				reviewed_by_user_id TEXT,
				status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
				rejection_reason TEXT
			);

			-- Enrolled OS nodes with certificates
			CREATE TABLE IF NOT EXISTS os_nodes (
				id TEXT PRIMARY KEY,
				certificate TEXT NOT NULL,
				certificate_fingerprint TEXT UNIQUE NOT NULL,
				certificate_cn TEXT NOT NULL,
				private_key_generated_by_node BOOLEAN DEFAULT 1,
				enrollment_request_id TEXT REFERENCES enrollment_requests(id),
				system_info TEXT NOT NULL,
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
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created OS node provisioning tables")
	}

	// Add storage tracking columns to storage_nodes
	var quotaMBExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('storage_nodes') WHERE name = 'quota_mb'`)
	if err := row.Scan(&quotaMBExists); err != nil {
		log.Printf("Warning: could not check for quota_mb column: %v", err)
	}

	if !quotaMBExists {
		log.Println("Adding storage tracking columns to storage_nodes table...")
		_, err := db.Exec(`ALTER TABLE storage_nodes ADD COLUMN quota_mb INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add quota_mb column: %v", err)
		}
		_, err = db.Exec(`ALTER TABLE storage_nodes ADD COLUMN used_mb INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add used_mb column: %v", err)
		}
		_, err = db.Exec(`ALTER TABLE storage_nodes ADD COLUMN available_mb INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add available_mb column: %v", err)
		}
		log.Println("✓ Migration complete: added storage tracking columns to storage_nodes table")
	}

	// Create file_shards table for Reed-Solomon erasure coding
	var fileShardsExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='file_shards'`)
	if err := row.Scan(&fileShardsExists); err != nil {
		log.Printf("Warning: could not check for file_shards table: %v", err)
	}

	if !fileShardsExists {
		log.Println("Creating file_shards table for Reed-Solomon erasure coding...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS file_shards (
				id TEXT PRIMARY KEY,
				file_id TEXT NOT NULL,
				shard_index INTEGER NOT NULL,
				shard_type TEXT NOT NULL CHECK (shard_type IN ('data', 'parity')),
				node_id TEXT NOT NULL,
				shard_size INTEGER NOT NULL,
				checksum TEXT,
				storage_path TEXT,
				status TEXT DEFAULT 'active' CHECK (status IN ('active', 'degraded', 'lost', 'recovering')),
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (file_id) REFERENCES files(id),
				FOREIGN KEY (node_id) REFERENCES storage_nodes(id),
				UNIQUE(file_id, shard_index)
			);

			CREATE INDEX IF NOT EXISTS idx_file_shards_file_id ON file_shards(file_id);
			CREATE INDEX IF NOT EXISTS idx_file_shards_node_id ON file_shards(node_id);
			CREATE INDEX IF NOT EXISTS idx_file_shards_status ON file_shards(status);
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created file_shards table")
	}

	// Add RS parameters to files table
	var rsDataShardsExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name = 'rs_data_shards'`)
	if err := row.Scan(&rsDataShardsExists); err != nil {
		log.Printf("Warning: could not check for rs_data_shards column: %v", err)
	}

	if !rsDataShardsExists {
		log.Println("Adding Reed-Solomon parameters to files table...")
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN rs_data_shards INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add rs_data_shards column: %v", err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN rs_parity_shards INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add rs_parity_shards column: %v", err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN total_shards INTEGER DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add total_shards column: %v", err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN is_erasure_coded BOOLEAN DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add is_erasure_coded column: %v", err)
		}
		log.Println("✓ Migration complete: added Reed-Solomon parameters to files table")
	}

	// Add peer file sharing settings table
	var peerSharingSettingsExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='peer_file_sharing_settings'`)
	if err := row.Scan(&peerSharingSettingsExists); err != nil {
		log.Printf("Warning: could not check for peer_file_sharing_settings table: %v", err)
	}

	if !peerSharingSettingsExists {
		log.Println("Creating peer file sharing tables...")
		_, err := db.Exec(`
			-- Peer file sharing settings
			CREATE TABLE IF NOT EXISTS peer_file_sharing_settings (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				enabled BOOLEAN DEFAULT 0,
				complete_storage_enabled BOOLEAN DEFAULT 0,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT DEFAULT CURRENT_TIMESTAMP
			);

			-- Insert default settings row
			INSERT INTO peer_file_sharing_settings (enabled, complete_storage_enabled) VALUES (0, 0);

			-- Peer file sharing outbound configuration
			CREATE TABLE IF NOT EXISTS peer_sharing_outbound (
				id TEXT PRIMARY KEY,
				peer_server_id TEXT NOT NULL,
				enabled BOOLEAN DEFAULT 1,
				priority INTEGER DEFAULT 0,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (peer_server_id) REFERENCES peer_servers(id),
				UNIQUE(peer_server_id)
			);

			-- Peer file sharing inbound configuration
			CREATE TABLE IF NOT EXISTS peer_sharing_inbound (
				id TEXT PRIMARY KEY,
				peer_server_id TEXT NOT NULL,
				enabled BOOLEAN DEFAULT 1,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (peer_server_id) REFERENCES peer_servers(id),
				UNIQUE(peer_server_id)
			);
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created peer file sharing tables")
	}

	// Add peer_server_id column to file_shards
	var peerServerIDExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('file_shards') WHERE name = 'peer_server_id'`)
	if err := row.Scan(&peerServerIDExists); err != nil {
		log.Printf("Warning: could not check for peer_server_id column in file_shards: %v", err)
	}

	if !peerServerIDExists {
		log.Println("Adding peer_server_id column to file_shards table...")
		_, err := db.Exec(`ALTER TABLE file_shards ADD COLUMN peer_server_id TEXT`)
		if err != nil {
			log.Printf("Warning: could not add peer_server_id column: %v", err)
		}
		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_file_shards_peer_server_id ON file_shards(peer_server_id)`)
		if err != nil {
			log.Printf("Warning: could not create peer_server_id index: %v", err)
		}
		log.Println("✓ Migration complete: added peer_server_id column to file_shards")
	}

	// Add source_server_id column to files
	var sourceServerIDExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name = 'source_server_id'`)
	if err := row.Scan(&sourceServerIDExists); err != nil {
		log.Printf("Warning: could not check for source_server_id column in files: %v", err)
	}

	if !sourceServerIDExists {
		log.Println("Adding source_server_id column to files table...")
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN source_server_id TEXT`)
		if err != nil {
			log.Printf("Warning: could not add source_server_id column: %v", err)
		}
		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_files_source_server_id ON files(source_server_id)`)
		if err != nil {
			log.Printf("Warning: could not create source_server_id index: %v", err)
		}
		log.Println("✓ Migration complete: added source_server_id column to files")
	}

	// Add storage_remaining column to peer_servers
	var storageRemainingExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('peer_servers') WHERE name = 'storage_remaining'`)
	if err := row.Scan(&storageRemainingExists); err != nil {
		log.Printf("Warning: could not check for storage_remaining column: %v", err)
	}

	if !storageRemainingExists {
		log.Println("Adding storage_remaining column to peer_servers table...")
		_, err := db.Exec(`ALTER TABLE peer_servers ADD COLUMN storage_remaining BIGINT DEFAULT 0`)
		if err != nil {
			log.Printf("Warning: could not add storage_remaining column: %v", err)
		}
		log.Println("✓ Migration complete: added storage_remaining column to peer_servers")
	}

	// Add trash columns to files table
	var filesDeletedAtExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name = 'deleted_at'`)
	if err := row.Scan(&filesDeletedAtExists); err != nil {
		log.Printf("Warning: could not check for deleted_at column in files: %v", err)
	}

	if !filesDeletedAtExists {
		log.Println("Adding trash columns to files table...")
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN deleted_at TEXT`)
		if err != nil {
			log.Printf("Warning: could not add deleted_at column to files: %v", err)
		}
		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_files_deleted_at ON files(deleted_at)`)
		if err != nil {
			log.Printf("Warning: could not create deleted_at index: %v", err)
		}
		log.Println("✓ Migration complete: added trash columns to files table")
	}

	// Add trash columns to folders table
	var foldersDeletedAtExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('folders') WHERE name = 'deleted_at'`)
	if err := row.Scan(&foldersDeletedAtExists); err != nil {
		log.Printf("Warning: could not check for deleted_at column in folders: %v", err)
	}

	if !foldersDeletedAtExists {
		log.Println("Adding trash columns to folders table...")
		_, err := db.Exec(`ALTER TABLE folders ADD COLUMN deleted_at TEXT`)
		if err != nil {
			log.Printf("Warning: could not add deleted_at column to folders: %v", err)
		}
		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_folders_deleted_at ON folders(deleted_at)`)
		if err != nil {
			log.Printf("Warning: could not create deleted_at index: %v", err)
		}
		log.Println("✓ Migration complete: added trash columns to folders table")
	}

	// Create pending_deletions table for offline node cleanup
	var pendingDeletionsExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='pending_deletions'`)
	if err := row.Scan(&pendingDeletionsExists); err != nil {
		log.Printf("Warning: could not check for pending_deletions table: %v", err)
	}

	if !pendingDeletionsExists {
		log.Println("Creating pending_deletions table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS pending_deletions (
				id TEXT PRIMARY KEY,
				file_id TEXT NOT NULL,
				shard_id TEXT,
				node_id TEXT,
				peer_server_id TEXT,
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

			CREATE INDEX IF NOT EXISTS idx_pending_deletions_node_id ON pending_deletions(node_id);
			CREATE INDEX IF NOT EXISTS idx_pending_deletions_peer_server_id ON pending_deletions(peer_server_id);
			CREATE INDEX IF NOT EXISTS idx_pending_deletions_status ON pending_deletions(status);
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created pending_deletions table")
	}

	// Add last_accessed column to files table
	var lastAccessedExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name = 'last_accessed'`)
	if err := row.Scan(&lastAccessedExists); err != nil {
		log.Printf("Warning: could not check for last_accessed column: %v", err)
	}

	if !lastAccessedExists {
		log.Println("Adding last_accessed column to files table...")
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN last_accessed TEXT`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: added last_accessed column to files table")
	}

	// Create favorites table
	var favoritesExists bool
	row = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='favorites'`)
	if err := row.Scan(&favoritesExists); err != nil {
		log.Printf("Warning: could not check for favorites table: %v", err)
	}

	if !favoritesExists {
		log.Println("Creating favorites table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS favorites (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				file_id TEXT,
				folder_id TEXT,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				CHECK (
					(file_id IS NOT NULL AND folder_id IS NULL) OR
					(file_id IS NULL AND folder_id IS NOT NULL)
				),
				FOREIGN KEY (user_id) REFERENCES users(id),
				FOREIGN KEY (file_id) REFERENCES files(id),
				FOREIGN KEY (folder_id) REFERENCES folders(id),
				UNIQUE(user_id, file_id),
				UNIQUE(user_id, folder_id)
			);

			CREATE INDEX IF NOT EXISTS idx_favorites_user_id ON favorites(user_id);
			CREATE INDEX IF NOT EXISTS idx_favorites_file_id ON favorites(file_id);
			CREATE INDEX IF NOT EXISTS idx_favorites_folder_id ON favorites(folder_id);
		`)
		if err != nil {
			return err
		}
		log.Println("✓ Migration complete: created favorites table")
	}

	return nil
}
