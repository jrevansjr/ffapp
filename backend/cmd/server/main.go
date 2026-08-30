// Command server starts the local HTTP API. It initializes the persistent
// SQLite store, including pending migrations, before accepting requests.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/jrevansjr/ffapp/backend/internal/api"
	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/draft"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

func main() {
	dbPath := database.PathFromEnv()
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("database ready at %s", dbPath)
	settings, err := database.GetSettings(context.Background(), db)
	if err != nil {
		log.Fatalf("load draft settings: %v", err)
	}
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	address := ":" + port
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("listen on %s: %v", address, err)
	}
	defer listener.Close()
	poller := draft.NewPoller(db, sleeper.NewClient())
	poller.Configure(settings)
	defer poller.Stop()

	log.Printf("starting API server on %s", address)
	server := &http.Server{Handler: api.NewRouter(db, poller.Configure)}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("API server stopped: %v", err)
	}
}
