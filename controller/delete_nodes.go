package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "/home/devat/TEST/controller/storageos.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM nodes")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("All entries from nodes table have been deleted.")
}