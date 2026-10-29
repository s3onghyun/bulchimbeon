package daemon

import (
	"path/filepath"
	"strings"
)

// Paths are the daemon's files (docs/compatibility.md §3).
type Paths struct {
	Settings string // $XDG_CONFIG_HOME/bulchimbeon/settings.json
	State    string // $XDG_STATE_HOME/bulchimbeon/state.json
	Log      string // $XDG_STATE_HOME/bulchimbeon/duty-log.txt
	Update   string // $XDG_STATE_HOME/bulchimbeon/update.json (the once-a-day update check)
}

// DefaultPaths follows the XDG Base Directory spec: settings under XDG_CONFIG_HOME
// (default ~/.config), the state file and the duty log under XDG_STATE_HOME (default
// ~/.local/state), where the spec puts logs. Relative XDG values are ignored, as the spec says.
func DefaultPaths(getenv func(string) string, home string) Paths {
	base := func(env, fallback string) string {
		if v := getenv(env); filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(home, fallback)
	}
	config := filepath.Join(base("XDG_CONFIG_HOME", ".config"), "bulchimbeon")
	state := filepath.Join(base("XDG_STATE_HOME", filepath.Join(".local", "state")), "bulchimbeon")
	return Paths{
		Settings: filepath.Join(config, "settings.json"),
		State:    filepath.Join(state, "state.json"),
		Log:      filepath.Join(state, "duty-log.txt"),
		Update:   filepath.Join(state, "update.json"),
	}
}

// Tilde shortens a path under home for display.
func Tilde(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
