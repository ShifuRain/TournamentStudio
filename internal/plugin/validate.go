package plugin

// Validate parses and loads a single plugin source in isolation --
// exercising the exact same sandboxed-exec-and-shape-check loadSource
// runs for every bundled and external file during Load -- without
// registering the result on any live Engine. Load's own external-file
// scan intentionally logs and skips a bad file rather than failing
// (correct for a background startup scan); Validate instead returns the
// error, for callers like an interactive plugin upload that need to
// tell the caller exactly what's wrong before persisting anything.
func Validate(name string, source []byte) error {
	scratch := &Engine{
		sports:          make(map[string]*SportPlugin),
		tournamentTypes: make(map[string]*TournamentTypePlugin),
	}
	if err := scratch.loadSource(name, source, name); err != nil {
		return err
	}
	scratch.Close()
	return nil
}
