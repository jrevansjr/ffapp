// Package api exposes the local JSON API and converts database models into
// stable response shapes for the React application.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jrevansjr/ffapp/backend/internal/database"
)

type healthResponse struct {
	Status string `json:"status"`
}

// handler gives every route access to the one database handle shared by the
// server. Subject-specific behavior stays in the corresponding API file.
type handler struct {
	db                *sql.DB
	onSettingsUpdated func(database.Settings)
}

// NewRouter connects the application's HTTP routes to the shared SQLite
// handle. Handlers only read or update local state; they never call Sleeper.
func NewRouter(db *sql.DB, onSettingsUpdated func(database.Settings)) http.Handler {
	h := handler{db: db, onSettingsUpdated: onSettingsUpdated}
	router := chi.NewRouter()
	router.Get("/api/health", h.handleHealth)
	router.Get("/api/nfl-teams", h.handleNFLTeams)
	router.Get("/api/players", h.handlePlayers)
	router.Get("/api/players/{id}", h.handlePlayer)
	router.Get("/api/settings", h.handleGetSettings)
	router.Put("/api/settings", h.handleUpdateSettings)
	router.Get("/api/draft/state", h.handleGetDraftState)
	return router
}

func (h handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// writeJSON is the common response boundary, ensuring successful and error
// responses use the same content type and encoding behavior.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode API response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}
