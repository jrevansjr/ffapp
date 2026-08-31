package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jrevansjr/ffapp/backend/internal/database"
)

type createManualPickRequest struct {
	PlayerID *int64 `json:"player_id"`
}

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

// handleCreateManualPick persists one fallback selection and returns the full
// updated local state so Draft Day can update without waiting for its next poll.
func (h handler) handleCreateManualPick(w http.ResponseWriter, r *http.Request) {
	var request createManualPickRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be a valid manual-pick object")
		return
	}
	if err := requireJSONEnd(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "request body must contain one manual-pick object")
		return
	}
	if request.PlayerID == nil || *request.PlayerID <= 0 {
		writeError(w, http.StatusBadRequest, "player_id must be a positive integer")
		return
	}

	pick, err := database.CreateManualPick(r.Context(), h.db, *request.PlayerID)
	if err != nil {
		handleManualPickError(w, err, "create")
		return
	}
	log.Printf(
		"manual draft pick created: pick=%d player_id=%d sleeper_player_id=%s",
		pick.PickNumber,
		*request.PlayerID,
		pick.SleeperPlayerID,
	)
	h.writeUpdatedDraftState(w, r, http.StatusCreated)
}

// handleDeleteManualPick undoes only a manual pick on the configured draft;
// official Sleeper picks remain read-only.
func (h handler) handleDeleteManualPick(w http.ResponseWriter, r *http.Request) {
	pickID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || pickID <= 0 {
		writeError(w, http.StatusBadRequest, "manual pick id must be a positive integer")
		return
	}
	pick, err := database.DeleteManualPick(r.Context(), h.db, pickID)
	if err != nil {
		handleManualPickError(w, err, "delete")
		return
	}
	log.Printf(
		"manual draft pick undone: pick=%d sleeper_player_id=%s",
		pick.PickNumber,
		pick.SleeperPlayerID,
	)
	h.writeUpdatedDraftState(w, r, http.StatusOK)
}

func (h handler) writeUpdatedDraftState(w http.ResponseWriter, r *http.Request, status int) {
	state, err := database.GetDraftState(r.Context(), h.db)
	if err != nil {
		log.Printf("reload draft state after manual action: %v", err)
		writeError(w, http.StatusInternalServerError, "manual action saved but draft state could not be reloaded")
		return
	}
	writeJSON(w, status, newDraftStateResponse(state))
}

func handleManualPickError(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, database.ErrDraftNotConfigured):
		writeError(w, http.StatusConflict, "configure a Sleeper draft ID before using manual picks")
	case errors.Is(err, database.ErrPlayerNotFound):
		writeError(w, http.StatusNotFound, "player not found")
	case errors.Is(err, database.ErrPlayerNotDraftable):
		writeError(w, http.StatusConflict, "player cannot be manually drafted without a Sleeper ID")
	case errors.Is(err, database.ErrPlayerAlreadyTaken):
		writeError(w, http.StatusConflict, "player is already taken in the active draft")
	case errors.Is(err, database.ErrManualPickNotFound):
		writeError(w, http.StatusNotFound, "manual pick not found in the active draft")
	default:
		log.Printf("%s manual draft pick: %v", action, err)
		writeError(w, http.StatusInternalServerError, "manual draft action failed")
	}
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
