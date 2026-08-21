package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jrevansjr/ffapp/backend/internal/api"
)

func main() {
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
