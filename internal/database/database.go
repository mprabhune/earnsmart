package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func InitDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to database")

	if err := applySchema(db); err != nil {
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}

	return db, nil
}

func applySchema(db *sql.DB) error {
	// Try current directory first (works in Docker where WORKDIR=/app),
	// then fallback for running from project root in local dev.
	paths := []string{"schema.sql", "../../schema.sql"}
	var schema []byte
	var err error
	for _, p := range paths {
		schema, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		// Schema file not found — skip silently (e.g. schema already applied externally)
		log.Println("Warning: schema.sql not found, skipping auto-apply")
		return nil
	}

	if _, err := db.Exec(string(schema)); err != nil {
		return fmt.Errorf("schema execution failed: %w", err)
	}

	log.Println("Database schema applied successfully")
	return nil
}
