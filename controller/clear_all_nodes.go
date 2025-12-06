package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Open the database
	db, err := sql.Open("sqlite3", "./storageos.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Tables to clear for complete enrollment reset
	tables := []string{
		"certificate_renewals",
		"certificate_revocations",
		"node_connections",
		"os_nodes",
		"enrollment_requests",
		"enrollment_otps",
		"storage_nodes", // Also clear storage nodes
	}

	for _, table := range tables {
		result, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			fmt.Printf("Error deleting from %s: %v\n", table, err)
		} else {
			rowsAffected, _ := result.RowsAffected()
			fmt.Printf("Deleted %d rows from %s\n", rowsAffected, table)
		}
	}

	fmt.Println("Database cleared of all enrollment and node data")
}