package plugin

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
)

type RosterField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
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
