package engine

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Kind identifies one agent. It is comparable so it can key the per-agent counts.
type Kind struct {
	ID          string
	DisplayName string
}

// AgentKind is one watched AI agent and the rules that recognise its processes
// (Swift `AgentKind`).
type AgentKind struct {
	Kind
	// BinaryNames are basenames of the executable, argv[0] or the runner's script (case-sensitive).
	BinaryNames []string
	// PathMarkers are install-path fragments, for executables named after a version number.
	PathMarkers []string
	// PackageMarkers are script-path fragments, for agents run by node, bun or deno.
	PackageMarkers []string
}

// ProcessSnapshot is what detection needs to know about one same-UID process.
type ProcessSnapshot struct {
	PID  int32
	PPID int32
	// ExecutablePath is the real executable (`/proc/<pid>/exe` on Linux).
	ExecutablePath string
	// Arguments holds argv[0] only, or the first 8 entries for script runners and the
	// first 64 for agent candidates. It is used in memory for matching and never stored.
	Arguments []string
	// CWD is collected on macOS for agent candidates only, for `--diagnose` output.
	CWD string
}

// Reason says which rule matched (diagnostic output).
type Reason int

const (
	ReasonExecutableName Reason = iota
	ReasonArgv0
	ReasonInstallPath
	ReasonScript
)

func (r Reason) String() string {
	switch r {
	case ReasonExecutableName:
		return "executableName"
	case ReasonArgv0:
		return "argv0"
	case ReasonInstallPath:
		return "installPath"
	default:
		return "script"
	}
}

// Label is the human-readable reason.
func (r Reason) Label(lang Lang) string {
	switch r {
	case ReasonExecutableName:
		return T(lang, "실행 파일 이름", "executable name", "可执行文件名")
	case ReasonArgv0:
		return T(lang, "실행 명령(argv[0])", "command (argv[0])", "命令 (argv[0])")
	case ReasonInstallPath:
		return T(lang, "설치 경로", "install path", "安装路径")
	default:
		return T(lang, "스크립트 경로", "script path", "脚本路径")
	}
}

// BuiltIn is the agent catalog. Keep it identical to `AgentCatalog.builtIn` in Swift;
// testdata/engine/catalog.json pins both.
var BuiltIn = []AgentKind{
	{Kind: Kind{"claude-code", "Claude Code"}, BinaryNames: []string{"claude"},
		// native install: the executable is named after the version, such as "2.1.296"
		PathMarkers: []string{"/.local/share/claude/versions/"}, PackageMarkers: []string{"@anthropic-ai/claude-code"}},
	{Kind: Kind{"codex", "Codex"}, BinaryNames: []string{"codex"}, PackageMarkers: []string{"@openai/codex"}},
	{Kind: Kind{"gemini-cli", "Gemini CLI"}, BinaryNames: []string{"gemini"}, PackageMarkers: []string{"@google/gemini-cli"}},
	{Kind: Kind{"cursor-agent", "Cursor Agent"}, BinaryNames: []string{"cursor-agent"}},
	{Kind: Kind{"copilot-cli", "Copilot CLI"}, BinaryNames: []string{"copilot"}, PackageMarkers: []string{"@github/copilot"}},
	{Kind: Kind{"aider", "Aider"}, BinaryNames: []string{"aider"}},
	{Kind: Kind{"opencode", "OpenCode"}, BinaryNames: []string{"opencode"}, PackageMarkers: []string{"opencode-ai"}},
}

var (
	scriptRunners   = []string{"node", "bun", "deno"}
	inlineEvalFlags = []string{"-e", "--eval", "-p", "--print"}
	customNameRE    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)
)

// Argument limits for the process scanner (same as the macOS app).
const (
	ArgsDefault   = 1  // argv[0] only
	ArgsRunner    = 8  // node, bun, deno
	ArgsCandidate = 64 // already an agent candidate: look for the helper-session flag
)

// MaxCustomAgents caps `customAgents`.
const MaxCustomAgents = 16

// NeedsFullArguments reports whether the executable is a script runner, the only case
// where more than argv[0] is read.
func NeedsFullArguments(executablePath string) bool {
	return slices.Contains(scriptRunners, Basename(executablePath))
}

// IsCandidate reports whether the executable or argv[0] already looks like an agent, so
// the scanner reads more arguments to rule out helper sessions.
func IsCandidate(executablePath string, argv0 *string, kinds []AgentKind) bool {
	exe := Basename(executablePath)
	for _, k := range kinds {
		if slices.Contains(k.BinaryNames, exe) || (argv0 != nil && slices.Contains(k.BinaryNames, Basename(*argv0))) {
			return true
		}
		if containsAny(executablePath, k.PathMarkers) {
			return true
		}
	}
	return false
}

// isHelperSession: a headless session with no tools (`--tools=`) is not an agent.
// Measured: the claude-mem plugin keeps one running for memory housekeeping.
func isHelperSession(arguments []string) bool {
	return len(arguments) > 1 && slices.Contains(arguments[1:], "--tools=")
}

// Match identifies a process as an agent. It never matches broad substrings, because
// hook scripts (`node -e "...'.claude'..."`) would turn into false positives.
func Match(p ProcessSnapshot, kinds []AgentKind) (AgentKind, Reason, bool) {
	if isHelperSession(p.Arguments) {
		return AgentKind{}, 0, false
	}
	exeBase := Basename(p.ExecutablePath)
	var argv0Base *string
	if len(p.Arguments) > 0 {
		b := Basename(p.Arguments[0])
		argv0Base = &b
	}
	for _, k := range kinds {
		if slices.Contains(k.BinaryNames, exeBase) {
			return k, ReasonExecutableName, true
		}
		if argv0Base != nil && slices.Contains(k.BinaryNames, *argv0Base) {
			return k, ReasonArgv0, true
		}
		if containsAny(p.ExecutablePath, k.PathMarkers) {
			return k, ReasonInstallPath, true
		}
	}

	if !slices.Contains(scriptRunners, exeBase) || len(p.Arguments) < 2 || slices.Contains(inlineEvalFlags, p.Arguments[1]) {
		return AgentKind{}, 0, false
	}
	script, found := "", false
	for _, a := range p.Arguments[1:] {
		if !strings.HasPrefix(a, "-") {
			script, found = a, true
			break
		}
	}
	if !found {
		return AgentKind{}, 0, false
	}
	// an npm global install runs through its bin link, such as `node /usr/local/bin/claude`
	scriptBase := Basename(script)
	for _, k := range kinds {
		if slices.Contains(k.BinaryNames, scriptBase) || containsAny(script, k.PackageMarkers) {
			return k, ReasonScript, true
		}
	}
	return AgentKind{}, 0, false
}

// Count returns the number of processes per agent.
func Count(processes []ProcessSnapshot, kinds []AgentKind) map[Kind]int {
	counts := map[Kind]int{}
	for _, p := range processes {
		if k, _, ok := Match(p, kinds); ok {
			counts[k.Kind]++
		}
	}
	return counts
}

// CustomKinds turns `customAgents` names into agent kinds. Names that break the naming
// rule are dropped, duplicates too, and at most MaxCustomAgents are kept.
func CustomKinds(names []string) []AgentKind {
	seen := map[string]bool{}
	var kinds []AgentKind
	for _, name := range names {
		if len(kinds) >= MaxCustomAgents || !customNameRE.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		kinds = append(kinds, AgentKind{Kind: Kind{"custom:" + name, name}, BinaryNames: []string{name}})
	}
	return kinds
}

// ValidCustomAgentName applies the `customAgents` naming rule.
// Settings front ends use it before storing a name.
func ValidCustomAgentName(name string) bool { return customNameRE.MatchString(name) }

// Summary is the short display form: "Claude Code ×3, Codex".
func Summary(counts map[Kind]int) string {
	var keys []Kind
	for k, n := range counts {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].DisplayName != keys[j].DisplayName {
			return keys[i].DisplayName < keys[j].DisplayName
		}
		return keys[i].ID < keys[j].ID
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		if counts[k] > 1 {
			parts[i] = k.DisplayName + " ×" + strconv.Itoa(counts[k])
		} else {
			parts[i] = k.DisplayName
		}
	}
	return strings.Join(parts, ", ")
}

// Basename matches Swift `AgentCatalog.basename`: the last non-empty path component, or
// the input itself when there is none.
func Basename(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
