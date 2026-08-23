package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/plugin"
)

type pluginSportResponse struct {
	ID                        string                `json:"id"`
	DisplayName               string                `json:"display_name"`
	CompatibleTournamentTypes []string              `json:"compatible_tournament_types"`
	RosterFields              []plugin.RosterField  `json:"roster_fields"`
}

type pluginTournamentTypeResponse struct {
	ID               string   `json:"id"`
	CompatibleSports []string `json:"compatible_sports"`
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	sports := make([]pluginSportResponse, 0)
	for _, sp := range s.plugins.Sports() {
		sports = append(sports, pluginSportResponse{
			ID:                        sp.ID,
			DisplayName:               sp.DisplayName,
			CompatibleTournamentTypes: sp.CompatibleTournamentTypes,
			RosterFields:              sp.RosterFields,
		})
	}

	tournamentTypes := make([]pluginTournamentTypeResponse, 0)
	for _, ttp := range s.plugins.TournamentTypes() {
		tournamentTypes = append(tournamentTypes, pluginTournamentTypeResponse{
			ID:               ttp.ID,
			CompatibleSports: ttp.CompatibleSports,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sports":           sports,
		"tournament_types": tournamentTypes,
	})
}
