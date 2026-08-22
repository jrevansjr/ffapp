package api

import (
	"log"
	"net/http"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

type nflTeamResponse struct {
	ID           int64  `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
}

// handleNFLTeams supplies the complete local NFL-team list used by frontend
// filters and display labels.
func (h handler) handleNFLTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := database.ListNFLTeams(r.Context(), h.db)
	if err != nil {
		log.Printf("list NFL teams: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load NFL teams")
		return
	}

	response := make([]nflTeamResponse, 0, len(teams))
	for _, team := range teams {
		response = append(response, nflTeamResponse(team))
	}
	writeJSON(w, http.StatusOK, response)
}
