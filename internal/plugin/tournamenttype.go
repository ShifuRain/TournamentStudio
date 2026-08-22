package plugin

import (
	"context"
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"

	"tournamentstudio/internal/ranking"
)

// pluginCallTimeout bounds how long a single Lua plugin call (
// next_round_groups or division_cuts) may run before it is aborted. Both
// calls are meant to be fast pure computation; a plugin that loops forever
// would otherwise hang the request goroutine and hold the plugin's mutex
// forever, blocking every subsequent call for every tournament using it.
//
// This is a var (not a const) so tests can temporarily shrink it to avoid
// a multi-second test for the infinite-loop case; production code must
// never change it.
var pluginCallTimeout = 5 * time.Second

// NextRoundGroups calls the plugin's next_round_groups(groups) function.
// groups[i] must already be sorted fastest-first (see ranking.Rank) —
// this method does not re-sort.
func (t *TournamentTypePlugin) NextRoundGroups(groups [][]ranking.TeamResult) ([][]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	L := t.state

	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	L.SetContext(ctx)
	defer L.RemoveContext()

	groupsTbl := L.NewTable()
	for _, group := range groups {
		groupTbl := L.NewTable()
		for _, r := range group {
			entry := L.NewTable()
			entry.RawSetString("team_id", lua.LString(r.TeamID))
			if r.TimeSeconds != nil {
				entry.RawSetString("time_seconds", lua.LNumber(*r.TimeSeconds))
			} else {
				entry.RawSetString("time_seconds", lua.LNil)
			}
			if r.Status != "" {
				entry.RawSetString("status", lua.LString(r.Status))
			} else {
				entry.RawSetString("status", lua.LNil)
			}
			groupTbl.Append(entry)
		}
		groupsTbl.Append(groupTbl)
	}

	if err := L.CallByParam(lua.P{
		Fn:      t.nextRoundGroupsFn,
		NRet:    1,
		Protect: true,
	}, groupsTbl); err != nil {
		return nil, fmt.Errorf("next_round_groups: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	retTbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("next_round_groups must return a table")
	}

	var result [][]string
	n := retTbl.Len()
	for i := 1; i <= n; i++ {
		groupTbl, ok := retTbl.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("next_round_groups result group %d is not a table", i)
		}
		var teamIDs []string
		m := groupTbl.Len()
		for j := 1; j <= m; j++ {
			idStr, ok := groupTbl.RawGetInt(j).(lua.LString)
			if !ok {
				return nil, fmt.Errorf("next_round_groups result group %d entry %d is not a string", i, j)
			}
			teamIDs = append(teamIDs, string(idStr))
		}
		result = append(result, teamIDs)
	}

	return result, nil
}
