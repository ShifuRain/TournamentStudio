package plugin

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

//go:embed bundled/*.lua
var bundledPlugins embed.FS

type Engine struct {
	sports          map[string]*SportPlugin
	tournamentTypes map[string]*TournamentTypePlugin
}

// Load registers the two plugins built into the binary (bundled/*.lua,
// embedded at compile time), then scans externalDir for additional or
// overriding *.lua files. A missing externalDir is not an error. A
// malformed or invalid external file is logged and skipped; a malformed
// bundled file is a hard error (it shipped broken, which is our bug, not
// a plugin author's).
func Load(externalDir string) (*Engine, error) {
	e := &Engine{
		sports:          make(map[string]*SportPlugin),
		tournamentTypes: make(map[string]*TournamentTypePlugin),
	}

	bundledEntries, err := bundledPlugins.ReadDir("bundled")
	if err != nil {
		return nil, fmt.Errorf("read bundled plugins: %w", err)
	}
	for _, entry := range bundledEntries {
		source, err := bundledPlugins.ReadFile("bundled/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read bundled plugin %s: %w", entry.Name(), err)
		}
		if err := e.loadSource(entry.Name(), source, "bundled"); err != nil {
			return nil, fmt.Errorf("bundled plugin %s: %w", entry.Name(), err)
		}
	}

	entries, err := os.ReadDir(externalDir)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(externalDir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		if err := e.loadSource(entry.Name(), source, entry.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
		}
	}

	return e, nil
}

// sandboxLibs is the set of gopher-lua standard libraries considered safe to
// expose to plugin code: they provide no filesystem, process, or module
// loading access. Notably absent: os, io, package/require, debug, coroutine,
// channel.
var sandboxLibs = []struct {
	name string
	fn   lua.LGFunction
}{
	{lua.BaseLibName, lua.OpenBase},
	{lua.TabLibName, lua.OpenTable},
	{lua.StringLibName, lua.OpenString},
	{lua.MathLibName, lua.OpenMath},
}

// dangerousGlobals are functions that gopher-lua's base library installs on
// _G unconditionally (regardless of whether the os/io/package libraries are
// separately opened). They allow executing arbitrary loaded code or pulling
// in modules, which defeats the sandbox, so they're removed after base is
// opened.
var dangerousGlobals = []string{
	"load",
	"loadstring",
	"loadfile",
	"dofile",
	"require",
	"module",
}

// newSandboxedState returns a Lua VM with only base, table, string, and math
// opened, and with load/dofile/require and friends removed from the global
// scope. os and io are never opened, so they're simply absent as globals.
// This is the only VM constructor plugin code ever runs in.
func newSandboxedState() *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	for _, lib := range sandboxLibs {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}

	for _, name := range dangerousGlobals {
		L.SetGlobal(name, lua.LNil)
	}

	return L
}

func (e *Engine) loadSource(name string, source []byte, pluginSource string) error {
	L := newSandboxedState()

	// Bound the top-level chunk execution the same way every subsequent
	// plugin call is bounded (see pluginCallTimeout in tournamenttype.go):
	// without this, a malicious or buggy plugin body (e.g. `while true do
	// end` at the top level, outside any function) runs before we ever get
	// a chance to inspect its returned table, and gopher-lua enforces no
	// bound of its own. This protects both Load's startup scan and
	// Validate's isolated upload-time check.
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	L.SetContext(ctx)
	defer L.RemoveContext()

	if err := L.DoString(string(source)); err != nil {
		L.Close()
		return fmt.Errorf("load %s: %w", name, err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	tbl, ok := ret.(*lua.LTable)
	if !ok {
		L.Close()
		return fmt.Errorf("%s: plugin file must return a table", name)
	}

	idVal, ok := tbl.RawGetString("id").(lua.LString)
	if !ok || string(idVal) == "" {
		L.Close()
		return fmt.Errorf("%s: plugin table must have a non-empty string 'id' field", name)
	}
	id := string(idVal)

	switch {
	case tbl.RawGetString("compatible_tournament_types") != lua.LNil:
		sp := parseSportPlugin(tbl, id)
		sp.Source = pluginSource
		e.sports[sp.ID] = sp
		L.Close()
		return nil
	case tbl.RawGetString("compatible_sports") != lua.LNil:
		ttp, err := parseTournamentTypePlugin(tbl, id, L)
		if err != nil {
			L.Close()
			return fmt.Errorf("%s: %w", name, err)
		}
		ttp.Source = pluginSource
		e.tournamentTypes[ttp.ID] = ttp
		return nil
	default:
		L.Close()
		return fmt.Errorf("%s: plugin table must have either 'compatible_tournament_types' or 'compatible_sports'", name)
	}
}

func (e *Engine) Sports() []*SportPlugin {
	result := make([]*SportPlugin, 0, len(e.sports))
	for _, sp := range e.sports {
		result = append(result, sp)
	}
	return result
}

func (e *Engine) TournamentTypes() []*TournamentTypePlugin {
	result := make([]*TournamentTypePlugin, 0, len(e.tournamentTypes))
	for _, ttp := range e.tournamentTypes {
		result = append(result, ttp)
	}
	return result
}

// Close releases the Lua VM held by every loaded tournament-type plugin.
// Sport plugins are pure metadata and their VM is already closed by the
// time Load returns.
func (e *Engine) Close() {
	for _, ttp := range e.tournamentTypes {
		if ttp.state != nil {
			ttp.state.Close()
		}
	}
}
