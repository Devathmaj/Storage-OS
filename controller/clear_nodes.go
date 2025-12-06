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

	// Delete all storage nodes
	result, err := db.Exec("DELETE FROM storage_nodes")
	if err != nil {
		log.Fatal(err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Deleted %d storage nodes\n", rowsAffected)

	// Also delete from nodes table if it exists
	result2, err := db.Exec("DELETE FROM nodes")
	if err != nil {
		fmt.Printf("Note: nodes table may not exist or error: %v\n", err)
	} else {
		rowsAffected2, _ := result2.RowsAffected()
		fmt.Printf("Deleted %d nodes\n", rowsAffected2)
	}

	// Also clear any related data like files, folders if needed for testing
	// But user specifically asked for nodes, so maybe just nodes

	fmt.Println("Database cleared of all nodes data")
}