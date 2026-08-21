package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"tournamentstudio/internal/importer"
	"tournamentstudio/internal/tournament"
)

func (s *Server) handleImportTeams(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	if _, err := s.tournaments.Get(tournamentID); err != nil {
		if err == tournament.ErrNotFound {
			http.Error(w, "tournament not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not look up tournament", http.StatusInternalServerError)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var rows []map[string]string
	switch {
	case strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx"):
		rows, err = importer.ParseXLSX(file)
	case strings.HasSuffix(strings.ToLower(header.Filename), ".csv"):
		rows, err = importer.ParseCSV(file)
	default:
		http.Error(w, "unsupported file type: use .csv or .xlsx", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "could not parse file: "+err.Error(), http.StatusBadRequest)
		return
	}

	result := importer.Validate(tournamentID, rows)
	imported := 0
	problems := result.Problems
	for i, t := range result.Teams {
		if _, err := s.teams.Create(t); err != nil {
			problems = append(problems, importer.RowProblem{
				RowIndex: i,
				Message:  "could not save team " + t.Name + ": " + err.Error(),
			})
			continue
		}
		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"imported": imported,
		"problems": problems,
	})
}
