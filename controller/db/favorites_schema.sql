-- Add favorites table
CREATE TABLE IF NOT EXISTS favorites (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    file_id TEXT REFERENCES files(id),
    folder_id TEXT REFERENCES folders(id),
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    CHECK ((file_id IS NOT NULL AND folder_id IS NULL) OR (file_id IS NULL AND folder_id IS NOT NULL)),
    UNIQUE(user_id, file_id),
    UNIQUE(user_id, folder_id)
);

CREATE INDEX IF NOT EXISTS idx_favorites_user_id ON favorites(user_id);
CREATE INDEX IF NOT EXISTS idx_favorites_file_id ON favorites(file_id);
CREATE INDEX IF NOT EXISTS idx_favorites_folder_id ON favorites(folder_id);

-- Add last_accessed to files table for recent functionality
-- Note: This should be added as a migration
-- ALTER TABLE files ADD COLUMN last_accessed TEXT;

CREATE INDEX IF NOT EXISTS idx_files_last_accessed ON files(last_accessed);
