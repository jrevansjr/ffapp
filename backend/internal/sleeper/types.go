// Package sleeper contains the small public-API client used by explicit data
// imports. Browser requests and backend startup never call Sleeper.
package sleeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// StringValue accepts the string, number, or null representations found in
// Sleeper's player metadata while preserving external identifiers as strings.
type StringValue struct {
	Value string
	Valid bool
}

// MarshalJSON emits the normalized representation used by deterministic
// client/importer fixtures.
func (value StringValue) MarshalJSON() ([]byte, error) {
	if !value.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(value.Value)
}

// UnmarshalJSON normalizes a JSON string or number into a Go string.
func (value *StringValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*value = StringValue{}
		return nil
	}
	if data[0] == '"' {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		value.Value = decoded
		value.Valid = decoded != ""
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("decode string-like value: %w", err)
	}
	value.Value = number.String()
	value.Valid = value.Value != ""
	return nil
}

// IntValue accepts integer fields that Sleeper may encode as either JSON
// numbers or numeric strings.
type IntValue struct {
	Value int
	Valid bool
}

// MarshalJSON emits a JSON integer or null.
func (value IntValue) MarshalJSON() ([]byte, error) {
	if !value.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(value.Value)
}

// UnmarshalJSON normalizes an integer-like value.
func (value *IntValue) UnmarshalJSON(data []byte) error {
	var stringValue StringValue
	if err := stringValue.UnmarshalJSON(data); err != nil {
		return err
	}
	if !stringValue.Valid {
		*value = IntValue{}
		return nil
	}
	parsed, err := strconv.Atoi(stringValue.Value)
	if err != nil {
		return fmt.Errorf("decode integer-like value %q: %w", stringValue.Value, err)
	}
	value.Value = parsed
	value.Valid = true
	return nil
}

// Player is the subset of Sleeper's NFL player object retained by the local
// identity table. The enclosing response map key remains authoritative for ID.
type Player struct {
	PlayerID              StringValue `json:"player_id"`
	FirstName             StringValue `json:"first_name"`
	LastName              StringValue `json:"last_name"`
	FullName              StringValue `json:"full_name"`
	Position              StringValue `json:"position"`
	Team                  StringValue `json:"team"`
	BirthDate             StringValue `json:"birth_date"`
	Active                bool        `json:"active"`
	Status                StringValue `json:"status"`
	Number                IntValue    `json:"number"`
	College               StringValue `json:"college"`
	Height                StringValue `json:"height"`
	Weight                StringValue `json:"weight"`
	BirthCountry          StringValue `json:"birth_country"`
	YearsExp              IntValue    `json:"years_exp"`
	DepthChartPosition    StringValue `json:"depth_chart_position"`
	DepthChartOrder       IntValue    `json:"depth_chart_order"`
	InjuryStatus          StringValue `json:"injury_status"`
	InjuryBodyPart        StringValue `json:"injury_body_part"`
	InjuryNotes           StringValue `json:"injury_notes"`
	InjuryStartDate       StringValue `json:"injury_start_date"`
	PracticeParticipation StringValue `json:"practice_participation"`
	ESPNID                StringValue `json:"espn_id"`
	SportradarID          StringValue `json:"sportradar_id"`
	RotowireID            StringValue `json:"rotowire_id"`
	RotoworldID           StringValue `json:"rotoworld_id"`
	YahooID               StringValue `json:"yahoo_id"`
	FantasyDataID         StringValue `json:"fantasy_data_id"`
	StatsID               StringValue `json:"stats_id"`
	GSISID                StringValue `json:"gsis_id"`
}

// PlayersResponse is keyed by Sleeper player ID.
type PlayersResponse map[string]Player

// DraftPick is the subset of a Sleeper draft-pick object needed for durable
// ordering, player identity mapping, and a useful fallback name when unknown.
type DraftPick struct {
	Round           int               `json:"round"`
	PickNumber      int               `json:"pick_no"`
	DraftSlot       int               `json:"draft_slot"`
	RosterID        StringValue       `json:"roster_id"`
	PickedBy        StringValue       `json:"picked_by"`
	SleeperPlayerID StringValue       `json:"player_id"`
	Metadata        DraftPickMetadata `json:"metadata"`
}

// DraftPickMetadata carries Sleeper's display identity for picks that do not
// yet map to a player in the local database.
type DraftPickMetadata struct {
	FirstName StringValue `json:"first_name"`
	LastName  StringValue `json:"last_name"`
	Position  StringValue `json:"position"`
	Team      StringValue `json:"team"`
}
