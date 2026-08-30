package api

import (
	"log"
	"net/http"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

type draftPickResponse struct {
	ID              int64   `json:"id"`
	PickNumber      int     `json:"pick_number"`
	Round           *int    `json:"round"`
	DraftSlot       *int    `json:"draft_slot"`
	RosterID        *string `json:"roster_id"`
	PickedBy        *string `json:"picked_by"`
	SleeperPlayerID string  `json:"sleeper_player_id"`
	PlayerID        *int64  `json:"player_id"`
	Source          string  `json:"source"`
	FirstName       *string `json:"first_name"`
	LastName        *string `json:"last_name"`
	Position        *string `json:"position"`
	Team            *string `json:"team"`
}

type draftStateResponse struct {
	DraftID                 string              `json:"draft_id"`
	Mode                    string              `json:"mode"`
	Status                  string              `json:"status"`
	PollingEnabled          bool                `json:"polling_enabled"`
	Stale                   bool                `json:"stale"`
	LastSyncedAt            *string             `json:"last_synced_at"`
	Message                 string              `json:"message"`
	Picks                   []draftPickResponse `json:"picks"`
	TakenPlayerIDs          []int64             `json:"taken_player_ids"`
	UnknownSleeperPlayerIDs []string            `json:"unknown_sleeper_player_ids"`
}

// handleGetDraftState serves SQLite only. Its one-second browser refresh never
// causes a Sleeper request; the independent backend poller owns that cadence.
func (h handler) handleGetDraftState(w http.ResponseWriter, r *http.Request) {
	state, err := database.GetDraftState(r.Context(), h.db)
	if err != nil {
		log.Printf("get draft state: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load draft state")
		return
	}
	writeJSON(w, http.StatusOK, newDraftStateResponse(state))
}

func newDraftStateResponse(state database.DraftState) draftStateResponse {
	response := draftStateResponse{
		DraftID:                 state.DraftID,
		Mode:                    state.Mode,
		PollingEnabled:          state.PollingEnabled,
		LastSyncedAt:            state.LastSyncedAt,
		Picks:                   make([]draftPickResponse, 0, len(state.Picks)),
		TakenPlayerIDs:          state.TakenPlayerIDs,
		UnknownSleeperPlayerIDs: state.UnknownSleeperIDs,
	}
	switch {
	case state.DraftID == "":
		response.Mode = "not_configured"
		response.Status = "not_configured"
		response.Message = "No active Sleeper draft is configured."
	case state.LastSyncError != nil:
		response.Status = "stale"
		response.Stale = true
		response.Message = *state.LastSyncError
	case !state.PollingEnabled:
		response.Status = "disabled"
		response.Message = "Draft polling is disabled; showing persisted draft state."
	case state.LastSyncedAt == nil:
		response.Status = "syncing"
		response.Stale = true
		response.Message = "Waiting for the first successful Sleeper sync."
	default:
		response.Status = "current"
	}
	for _, pick := range state.Picks {
		response.Picks = append(response.Picks, draftPickResponse{
			ID:              pick.ID,
			PickNumber:      pick.PickNumber,
			Round:           pick.Round,
			DraftSlot:       pick.DraftSlot,
			RosterID:        pick.RosterID,
			PickedBy:        pick.PickedBy,
			SleeperPlayerID: pick.SleeperPlayerID,
			PlayerID:        pick.PlayerID,
			Source:          pick.Source,
			FirstName:       pick.PlayerFirstName,
			LastName:        pick.PlayerLastName,
			Position:        pick.PlayerPosition,
			Team:            pick.PlayerTeam,
		})
	}
	return response
}
