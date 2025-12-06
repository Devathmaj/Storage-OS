package models

import (
	"database/sql"
	"time"
)

// File represents a file in the system
type File struct {
	ID                string  `json:"id"`
	OwnerID           string  `json:"owner_id"`
	FolderID          *string `json:"folder_id,omitempty"`
	Filename          string  `json:"filename"`
	Extension         string  `json:"extension"`
	OriginalExtension string  `json:"original_extension"`
	MimeType          string  `json:"mime_type"`
	Size              int64   `json:"size"`
	OriginalSize      int64   `json:"original_size"`
	Checksum          string  `json:"checksum"`
	StoragePath       string  `json:"storage_path"`      // encrypted file path
	EncryptedKey      []byte  `json:"encrypted_key"`     // encrypted AES key
	EphemeralPubKey   []byte  `json:"ephemeral_pub_key"` // ephemeral public key
	NodeID            string  `json:"node_id"`           // which node stores it (leave empty for now)
	Version           int     `json:"version"`
	Status            string  `json:"status"`
	IsIndexed         bool    `json:"is_indexed"`
	ParentArchiveID   *string `json:"parent_archive_id,omitempty"`
	RelativePath      string  `json:"relative_path,omitempty"`
	RSDataShards      int     `json:"rs_data_shards"`      // Reed-Solomon data shard count
	RSParityShards    int     `json:"rs_parity_shards"`    // Reed-Solomon parity shard count
	TotalShards       int     `json:"total_shards"`        // Total shard count (data + parity)
	IsErasureCoded    bool    `json:"is_erasure_coded"`    // Whether file uses RS encoding
	UploadedAt        string  `json:"uploaded_at"`
	ModifiedAt        string  `json:"modified_at"`
}

// Folder represents a folder in the system
type Folder struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	ParentID      *string   `json:"parent_id,omitempty"`
	Name          string    `json:"name"`
	IsArchived    bool      `json:"is_archived"`
	ArchiveFileID *string   `json:"archive_file_id,omitempty"`
	TotalFiles    int       `json:"total_files"`
	TotalSize     int64     `json:"total_size"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateFile inserts a new file into the database
func CreateFile(db *sql.DB, file *File) error {
	query := `
		INSERT INTO files (id, owner_id, folder_id, filename, extension, original_extension, mime_type, size, original_size, checksum, storage_path, encrypted_key, ephemeral_pub_key, node_id, version, status, is_indexed, parent_archive_id, relative_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, file.ID, file.OwnerID, file.FolderID, file.Filename, file.Extension, file.OriginalExtension,
		file.MimeType, file.Size, file.OriginalSize, file.Checksum, file.StoragePath, file.EncryptedKey, file.EphemeralPubKey,
		file.NodeID, file.Version, file.Status, file.IsIndexed, file.ParentArchiveID, file.RelativePath)
	return err
}

// UpdateFile updates an existing file in the database
func UpdateFile(db *sql.DB, file *File) error {
	query := `
		UPDATE files 
		SET owner_id = ?, folder_id = ?, filename = ?, extension = ?, original_extension = ?, mime_type = ?, 
		    size = ?, original_size = ?, checksum = ?, storage_path = ?, encrypted_key = ?, ephemeral_pub_key = ?, 
		    node_id = ?, version = ?, status = ?, is_indexed = ?, parent_archive_id = ?, relative_path = ?, 
		    modified_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := db.Exec(query, file.OwnerID, file.FolderID, file.Filename, file.Extension, file.OriginalExtension,
		file.MimeType, file.Size, file.OriginalSize, file.Checksum, file.StoragePath, file.EncryptedKey, file.EphemeralPubKey,
		file.NodeID, file.Version, file.Status, file.IsIndexed, file.ParentArchiveID, file.RelativePath, file.ID)
	return err
}

// CreateFolder inserts a new folder into the database
func CreateFolder(db *sql.DB, folder *Folder) error {
	query := `
		INSERT INTO folders (id, owner_id, parent_id, name, is_archived, archive_file_id, total_files, total_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, folder.ID, folder.OwnerID, folder.ParentID, folder.Name, folder.IsArchived, folder.ArchiveFileID, folder.TotalFiles, folder.TotalSize)
	return err
}

// GetFilesByFolderID retrieves all files in a folder
func GetFilesByFolderID(db *sql.DB, folderID string) ([]*File, error) {
	query := `
		SELECT id, owner_id, folder_id, filename, extension, original_extension, mime_type, size, original_size, checksum, storage_path, encrypted_key, ephemeral_pub_key, node_id, version, status, is_indexed, parent_archive_id, relative_path, uploaded_at, modified_at
		FROM files WHERE folder_id = ? AND status = 'active'
	`
	rows, err := db.Query(query, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		var folderID sql.NullString
		var parentArchiveID sql.NullString
		var relativePath sql.NullString
		err := rows.Scan(
			&file.ID,
			&file.OwnerID,
			&folderID,
			&file.Filename,
			&file.Extension,
			&file.OriginalExtension,
			&file.MimeType,
			&file.Size,
			&file.OriginalSize,
			&file.Checksum,
			&file.StoragePath,
			&file.EncryptedKey,
			&file.EphemeralPubKey,
			&file.NodeID,
			&file.Version,
			&file.Status,
			&file.IsIndexed,
			&parentArchiveID,
			&relativePath,
			&file.UploadedAt,
			&file.ModifiedAt,
		)
		if err != nil {
			return nil, err
		}
		if folderID.Valid {
			file.FolderID = &folderID.String
		}
		if parentArchiveID.Valid {
			file.ParentArchiveID = &parentArchiveID.String
		}
		if relativePath.Valid {
			file.RelativePath = relativePath.String
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// GetFileByID retrieves a file by ID
func GetFileByID(db *sql.DB, id string) (*File, error) {
	file := &File{}
	query := `
		SELECT id, owner_id, folder_id, filename, extension, original_extension, mime_type, size, original_size, checksum, storage_path, encrypted_key, ephemeral_pub_key, node_id, version, status, is_indexed, parent_archive_id, relative_path, uploaded_at, modified_at
		FROM files WHERE id = ?
	`
	var folderID sql.NullString
	var parentArchiveID sql.NullString
	var relativePath sql.NullString
	err := db.QueryRow(query, id).Scan(
		&file.ID,
		&file.OwnerID,
		&folderID,
		&file.Filename,
		&file.Extension,
		&file.OriginalExtension,
		&file.MimeType,
		&file.Size,
		&file.OriginalSize,
		&file.Checksum,
		&file.StoragePath,
		&file.EncryptedKey,
		&file.EphemeralPubKey,
		&file.NodeID,
		&file.Version,
		&file.Status,
		&file.IsIndexed,
		&parentArchiveID,
		&relativePath,
		&file.UploadedAt,
		&file.ModifiedAt,
	)
	if err != nil {
		return nil, err
	}
	if folderID.Valid {
		file.FolderID = &folderID.String
	}
	if parentArchiveID.Valid {
		file.ParentArchiveID = &parentArchiveID.String
	}
	if relativePath.Valid {
		file.RelativePath = relativePath.String
	}
	return file, nil
}

// GetFilesByOwnerID retrieves all files for an owner
func GetFilesByOwnerID(db *sql.DB, ownerID string) ([]*File, error) {
	query := `
		SELECT id, owner_id, folder_id, filename, extension, mime_type, size, checksum, storage_path, encrypted_key, ephemeral_pub_key, node_id, version, status, uploaded_at, modified_at
		FROM files WHERE owner_id = ? AND status = 'active'
	`
	rows, err := db.Query(query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		var folderID sql.NullString
		err := rows.Scan(&file.ID, &file.OwnerID, &folderID, &file.Filename, &file.Extension,
			&file.MimeType, &file.Size, &file.Checksum, &file.StoragePath, &file.EncryptedKey, &file.EphemeralPubKey, &file.NodeID,
			&file.Version, &file.Status, &file.UploadedAt, &file.ModifiedAt)
		if err != nil {
			return nil, err
		}
		if folderID.Valid {
			file.FolderID = &folderID.String
		}
		files = append(files, file)
	}
	return files, rows.Err()
}
