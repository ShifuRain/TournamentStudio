package plugin

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func parseSportPlugin(tbl *lua.LTable, id string) *SportPlugin {
	sp := &SportPlugin{ID: id}

	if dn, ok := tbl.RawGetString("display_name").(lua.LString); ok {
		sp.DisplayName = string(dn)
	}

	if ctt, ok := tbl.RawGetString("compatible_tournament_types").(*lua.LTable); ok {
		ctt.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				sp.CompatibleTournamentTypes = append(sp.CompatibleTournamentTypes, string(s))
			}
		})
	}

	if rf, ok := tbl.RawGetString("roster_fields").(*lua.LTable); ok {
		rf.ForEach(func(_, v lua.LValue) {
			fieldTbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			field := RosterField{}
			if key, ok := fieldTbl.RawGetString("key").(lua.LString); ok {
				field.Key = string(key)
			}
			if label, ok := fieldTbl.RawGetString("label").(lua.LString); ok {
				field.Label = string(label)
			}
			if req, ok := fieldTbl.RawGetString("required").(lua.LBool); ok {
				field.Required = bool(req)
			}
			sp.RosterFields = append(sp.RosterFields, field)
		})
	}

	return sp
}

func parseTournamentTypePlugin(tbl *lua.LTable, id string, L *lua.LState) (*TournamentTypePlugin, error) {
	ttp := &TournamentTypePlugin{ID: id, state: L}

	if cs, ok := tbl.RawGetString("compatible_sports").(*lua.LTable); ok {
		cs.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				ttp.CompatibleSports = append(ttp.CompatibleSports, string(s))
			}
		})
	}

	nextFn, ok := tbl.RawGetString("next_round_groups").(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("must define next_round_groups")
	}
	ttp.nextRoundGroupsFn = nextFn

	divFn, ok := tbl.RawGetString("division_cuts").(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("must define division_cuts")
	}
	ttp.divisionCutsFn = divFn

	return ttp, nil
}
