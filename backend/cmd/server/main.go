// Command server starts the local HTTP API. It initializes the persistent
// SQLite store, including pending migrations, before accepting requests.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jrevansjr/ffapp/backend/internal/api"
	"github.com/jrevansjr/ffapp/backend/internal/database"
)

func main() {
	dbPath := database.PathFromEnv()
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("database ready at %s", dbPath)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	address := ":" + port
	log.Printf("starting API server on %s", address)
	if err := http.ListenAndServe(address, api.NewRouter()); err != nil {
		log.Fatalf("API server stopped: %v", err)
	}
}
