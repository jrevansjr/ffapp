// Command seed loads the deterministic development dataset into the same
// SQLite store used by the API server. It is an explicit, repeatable action.
package main

import (
	"context"
	"log"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

func main() {
	dbPath := database.PathFromEnv()
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer db.Close()

	if err := database.SeedSampleData(context.Background(), db); err != nil {
		log.Fatalf("seed sample data: %v", err)
	}
	log.Printf("sample data loaded into %s", dbPath)
}
