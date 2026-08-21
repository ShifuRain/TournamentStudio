package plugin

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
)

type RosterField struct {
	Key      string
	Label    string
	Required bool
}

type SportPlugin struct {
	ID                        string
	DisplayName               string
	CompatibleTournamentTypes []string
	RosterFields              []RosterField
}

type TournamentTypePlugin struct {
	ID               string
	CompatibleSports []string

	state             *lua.LState
	mu                sync.Mutex
	nextRoundGroupsFn *lua.LFunction
	divisionCutsFn    *lua.LFunction
}
