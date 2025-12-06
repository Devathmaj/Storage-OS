package db

import (
    "context"
    "database/sql"

    _ "embed"
)

//go:embed schema.sql
var schemaSQL string

// ApplySchema ensures the core metadata tables exist.
func ApplySchema(ctx context.Context, conn *sql.DB) error {
    _, err := conn.ExecContext(ctx, schemaSQL)
    return err
}
