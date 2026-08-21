package plugin

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

type Cut struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type Division struct {
	Name    string   `json:"name"`
	TeamIDs []string `json:"team_ids"`
}

// DivisionCuts calls the plugin's division_cuts(ranked_teams, cuts)
// function. rankedTeamIDs must already be in final rank order (fastest
// first) — this method does not sort.
func (t *TournamentTypePlugin) DivisionCuts(rankedTeamIDs []string, cuts []Cut) ([]Division, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	L := t.state

	rankedTbl := L.NewTable()
	for _, id := range rankedTeamIDs {
		rankedTbl.Append(lua.LString(id))
	}

	cutsTbl := L.NewTable()
	for _, c := range cuts {
		cutTbl := L.NewTable()
		cutTbl.RawSetString("name", lua.LString(c.Name))
		cutTbl.RawSetString("size", lua.LNumber(c.Size))
		cutsTbl.Append(cutTbl)
	}

	if err := L.CallByParam(lua.P{
		Fn:      t.divisionCutsFn,
		NRet:    1,
		Protect: true,
	}, rankedTbl, cutsTbl); err != nil {
		return nil, fmt.Errorf("division_cuts: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	retTbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("division_cuts must return a table")
	}

	var divisions []Division
	n := retTbl.Len()
	for i := 1; i <= n; i++ {
		divTbl, ok := retTbl.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("division_cuts result entry %d is not a table", i)
		}

		nameVal := divTbl.RawGetString("name")
		name, ok := nameVal.(lua.LString)
		if !ok {
			return nil, fmt.Errorf("division_cuts result entry %d has no name", i)
		}

		teamIDsTbl, ok := divTbl.RawGetString("team_ids").(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("division_cuts result entry %d has no team_ids table", i)
		}

		var teamIDs []string
		m := teamIDsTbl.Len()
		for j := 1; j <= m; j++ {
			idStr, ok := teamIDsTbl.RawGetInt(j).(lua.LString)
			if !ok {
				return nil, fmt.Errorf("division_cuts result entry %d team_ids entry %d is not a string", i, j)
			}
			teamIDs = append(teamIDs, string(idStr))
		}

		divisions = append(divisions, Division{Name: string(name), TeamIDs: teamIDs})
	}

	return divisions, nil
}
