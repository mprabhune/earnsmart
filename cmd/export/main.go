// Command export writes a JSON snapshot of family data for one-time import into
// the local-first native (Flutter) app.
//
//	DATABASE_URL=postgres://... go run ./cmd/export -out earnsmart_export.json
//
// Flags:
//
//	-out     output file (default earnsmart_export.json)
//	-email   export only the family owning this parent email
//	-family  export only this family id
//	-all     export every family (default when neither -email nor -family given)
//
// With a single family selected the file is one bundle object; otherwise it is
// {"exported_at":..., "families":[bundle, ...]}.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"earnsmart/internal/exporter"

	_ "github.com/lib/pq"
)

func main() {
	out := flag.String("out", "earnsmart_export.json", "output file path")
	email := flag.String("email", "", "export only the family owning this parent email")
	familyID := flag.String("family", "", "export only this family id")
	all := flag.Bool("all", false, "export every family")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	// Resolve a target family id from -email if given.
	if *email != "" && *familyID == "" {
		if err := db.QueryRow(
			`SELECT family_id FROM profiles WHERE email = $1 AND role = 'parent'`, *email,
		).Scan(familyID); err != nil {
			log.Fatalf("no parent found for email %q: %v", *email, err)
		}
	}

	var payload any
	if *familyID != "" {
		bundle, err := exporter.ExportFamily(db, *familyID)
		if err != nil {
			log.Fatalf("export family %s: %v", *familyID, err)
		}
		payload = bundle
		log.Printf("exported family %s: %d profiles, %d task defs, %d task logs, %d ledger rows",
			*familyID, len(bundle.Profiles), len(bundle.TaskDefinitions), len(bundle.TaskLogs), len(bundle.Ledger))
	} else {
		if !*all {
			log.Println("no -email / -family given; exporting ALL families")
		}
		ids, err := exporter.ListFamilyIDs(db)
		if err != nil {
			log.Fatalf("list families: %v", err)
		}
		bundles := make([]*exporter.Bundle, 0, len(ids))
		for _, id := range ids {
			b, err := exporter.ExportFamily(db, id)
			if err != nil {
				log.Fatalf("export family %s: %v", id, err)
			}
			bundles = append(bundles, b)
			log.Printf("exported family %s (%s): %d profiles, %d task logs",
				id, b.Family.FamilyName, len(b.Profiles), len(b.TaskLogs))
		}
		payload = map[string]any{
			"exported_at":    time.Now().UTC(),
			"schema_version": exporter.SchemaVersion,
			"families":       bundles,
		}
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create %s: %v", *out, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		log.Fatalf("write json: %v", err)
	}

	info, _ := os.Stat(*out)
	fmt.Printf("wrote %s (%d bytes)\n", *out, info.Size())
}
