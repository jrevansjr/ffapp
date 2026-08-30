package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

const (
	minimumPollingInterval = 500
	maximumPollingInterval = 60_000
)

type settingsResponse struct {
	SleeperUsername string  `json:"sleeper_username"`
	SleeperLeagueID string  `json:"sleeper_league_id"`
	SleeperDraftID  string  `json:"sleeper_draft_id"`
	PollingEnabled  bool    `json:"polling_enabled"`
	PollingInterval int     `json:"polling_interval_ms"`
	PlayersSyncedAt *string `json:"players_synced_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// Pointer fields distinguish a missing required JSON property from a supplied
// zero value such as an empty ID or disabled polling.
type updateSettingsRequest struct {
	SleeperUsername *string `json:"sleeper_username"`
	SleeperLeagueID *string `json:"sleeper_league_id"`
	SleeperDraftID  *string `json:"sleeper_draft_id"`
	PollingEnabled  *bool   `json:"polling_enabled"`
	PollingInterval *int    `json:"polling_interval_ms"`
}

func (h handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := database.GetSettings(r.Context(), h.db)
	if err != nil {
		log.Printf("get settings: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	writeJSON(w, http.StatusOK, newSettingsResponse(settings))
}

// handleUpdateSettings treats PUT as full replacement of editable fields. Sync
// metadata is not accepted from the browser and is preserved by the database.
func (h handler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var request updateSettingsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be a valid settings object")
		return
	}
	if err := requireJSONEnd(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "request body must contain one settings object")
		return
	}

	input, validationMessage := request.validate()
	if validationMessage != "" {
		writeError(w, http.StatusBadRequest, validationMessage)
		return
	}
	settings, err := database.UpdateSettings(r.Context(), h.db, input)
	if err != nil {
		log.Printf("update settings: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	log.Printf("settings updated")
	if h.onSettingsUpdated != nil {
		h.onSettingsUpdated(settings)
	}
	writeJSON(w, http.StatusOK, newSettingsResponse(settings))
}

// validate enforces the complete Admin form contract and trims human-entered
// identifiers without requiring them during initial setup.
func (request updateSettingsRequest) validate() (database.EditableSettings, string) {
	if request.SleeperUsername == nil || request.SleeperLeagueID == nil ||
		request.SleeperDraftID == nil || request.PollingEnabled == nil ||
		request.PollingInterval == nil {
		return database.EditableSettings{}, "all settings fields are required"
	}
	if *request.PollingInterval < minimumPollingInterval ||
		*request.PollingInterval > maximumPollingInterval {
		return database.EditableSettings{}, "polling_interval_ms must be between 500 and 60000"
	}
	return database.EditableSettings{
		SleeperUsername: strings.TrimSpace(*request.SleeperUsername),
		SleeperLeagueID: strings.TrimSpace(*request.SleeperLeagueID),
		SleeperDraftID:  strings.TrimSpace(*request.SleeperDraftID),
		PollingEnabled:  *request.PollingEnabled,
		PollingInterval: *request.PollingInterval,
	}, ""
}

func newSettingsResponse(settings database.Settings) settingsResponse {
	return settingsResponse{
		SleeperUsername: settings.SleeperUsername,
		SleeperLeagueID: settings.SleeperLeagueID,
		SleeperDraftID:  settings.SleeperDraftID,
		PollingEnabled:  settings.PollingEnabled,
		PollingInterval: settings.PollingInterval,
		PlayersSyncedAt: settings.PlayersSyncedAt,
		UpdatedAt:       settings.UpdatedAt,
	}
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("unexpected second JSON value")
	}
	return err
}
