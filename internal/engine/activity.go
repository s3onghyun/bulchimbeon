package engine

import "time"

// Whether an agent is merely open or actually working (Swift `AgentActivity`).
//
// Measured on macOS (2026-10): of 7 open Claude Code sessions only the one at work
// spawned `caffeinate -i -t 300` and used about 10% CPU; the others sat at 0-0.1%.
//
// False positive (2026-11, 0.12.0): idle Claude Code sessions kept the Mac awake. Their
// long-lived descendants (Stop-hook pollers, MCP servers) each used about 1% and added
// up past the threshold, a turn held open by an hours-long Stop hook kept Claude Code's
// own caffeinate running, and headless `claude -p` helpers launched by hooks counted as
// agents and added their CPU to the parent session.
//
// So working means the agent's own recent activity:
//   - the agent plus descendants started within BackgroundAge use at least CPUThreshold;
//   - an older descendant counts only when it alone uses BackgroundCPUThreshold (a long build);
//   - a caffeinate child counts only next to another child started within BackgroundAge
//     (Claude Code's Bash tool and command hooks time out after 10 minutes by default);
//   - an agent launched under another agent is judged on its own and not added to it.
const (
	CPUThreshold = 2.0 // percent
	// BackgroundAge: descendants running longer than this are background processes.
	BackgroundAge = 10 * time.Minute
	// BackgroundCPUThreshold: a background process counts only when it alone uses this
	// much (measured: Stop-hook poller ~1%, MCP server ~0.3%, a build 85%).
	BackgroundCPUThreshold = 5.0 // percent
	// BusyGrace mirrors Swift `AgentActivity.busyGrace`; the engine uses its own GraceMinutes.
	BusyGrace = 5 * time.Minute
	// maxTreeVisit bounds the descendant walk (cyclic parent links must not hang it).
	maxTreeVisit = 512
)

// Status counts open and working processes of one agent.
type Status struct {
	Present int
	Busy    int
}

// BusyReason says why an agent is working (the first rule that matched).
type BusyReason int

const (
	NotBusy BusyReason = iota
	BusyCaffeinate
	BusyRecentCPU
	BusyBackground
)

// String returns the Swift `AgentActivity.Reason` raw value ("" when not busy).
func (r BusyReason) String() string {
	switch r {
	case BusyCaffeinate:
		return "caffeinate"
	case BusyRecentCPU:
		return "recentCPU"
	case BusyBackground:
		return "background"
	}
	return ""
}

// Contributor is one descendant, for diagnostics. Age is negative when the start time is unknown.
type Contributor struct {
	PID            int32
	ExecutablePath string
	CPU            float64
	Age            time.Duration
}

// Explanation is why one agent is or is not working. It holds no arguments or environment.
type Explanation struct {
	Reason BusyReason
	// Caffeinate is the PID of a caffeinate child (0 = none).
	Caffeinate int32
	// FreshChild is the recently started child that backs the caffeinate signal.
	FreshChild *Contributor
	OwnCPU     float64
	// RecentCPU is the agent's CPU plus that of descendants started within BackgroundAge.
	RecentCPU float64
	// TopRecent and TopBackground are the busiest recent and long-running descendants.
	TopRecent     *Contributor
	TopBackground *Contributor
	// ParentAgent is the nearest ancestor that is an agent (0 = none).
	ParentAgent int32
}

// Busy reports whether the agent is working.
func (e Explanation) Busy() bool { return e.Reason != NotBusy }

// Tree indexes processes by parent and records which of them are agents.
type Tree struct {
	Children map[int32][]ProcessSnapshot
	ByPID    map[int32]ProcessSnapshot
	Agents   map[int32]AgentKind
}

// NewTree builds the process tree, keeping input order among siblings.
func NewTree(processes []ProcessSnapshot, kinds []AgentKind) Tree {
	t := Tree{Children: Children(processes), ByPID: map[int32]ProcessSnapshot{}, Agents: map[int32]AgentKind{}}
	for _, p := range processes {
		t.ByPID[p.PID] = p
		if k, _, ok := Match(p, kinds); ok {
			t.Agents[p.PID] = k
		}
	}
	return t
}

// Evaluate returns the open and working counts per agent.
func Evaluate(processes []ProcessSnapshot, cpu map[int32]float64, now time.Time, kinds []AgentKind) map[Kind]Status {
	tree := NewTree(processes, kinds)
	result := map[Kind]Status{}
	for _, p := range processes {
		k, ok := tree.Agents[p.PID]
		if !ok {
			continue
		}
		s := result[k.Kind]
		s.Present++
		if Explain(p, tree, cpu, now).Busy() {
			s.Busy++
		}
		result[k.Kind] = s
	}
	return result
}

// Children indexes processes by parent PID, keeping input order.
func Children(processes []ProcessSnapshot) map[int32][]ProcessSnapshot {
	children := map[int32][]ProcessSnapshot{}
	for _, p := range processes {
		children[p.PPID] = append(children[p.PPID], p)
	}
	return children
}

// Explain decides whether one agent process is working, and why.
func Explain(agent ProcessSnapshot, tree Tree, cpu map[int32]float64, now time.Time) Explanation {
	e := Explanation{ParentAgent: parentAgent(agent, tree), OwnCPU: cpu[agent.PID]}
	e.RecentCPU = e.OwnCPU

	contributor := func(p ProcessSnapshot) Contributor {
		c := Contributor{PID: p.PID, ExecutablePath: p.ExecutablePath, CPU: cpu[p.PID], Age: -1}
		if !p.StartedAt.IsZero() {
			c.Age = max(0, now.Sub(p.StartedAt))
		}
		return c
	}
	recent := func(c Contributor) bool { return c.Age < BackgroundAge }
	more := func(c Contributor, old *Contributor) bool {
		if old == nil {
			return c.CPU > 0
		}
		return c.CPU > old.CPU
	}

	for _, child := range tree.Children[agent.PID] {
		if child.PID == agent.PID {
			continue
		}
		if Basename(child.ExecutablePath) == "caffeinate" {
			if e.Caffeinate == 0 {
				e.Caffeinate = child.PID
			}
		} else if _, isAgent := tree.Agents[child.PID]; e.FreshChild == nil && !isAgent {
			if c := contributor(child); recent(c) {
				e.FreshChild = &c
			}
		}
	}

	// Descendants: the walk stops at other agents, which are judged on their own.
	queue := append([]ProcessSnapshot(nil), tree.Children[agent.PID]...)
	visited := map[int32]bool{agent.PID: true}
	for len(queue) > 0 {
		next := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if len(visited) >= maxTreeVisit {
			break
		}
		if visited[next.PID] {
			continue
		}
		visited[next.PID] = true
		if _, isAgent := tree.Agents[next.PID]; isAgent {
			continue
		}
		c := contributor(next)
		if recent(c) {
			e.RecentCPU += c.CPU
			if more(c, e.TopRecent) {
				e.TopRecent = &c
			}
		} else if more(c, e.TopBackground) {
			e.TopBackground = &c
		}
		queue = append(queue, tree.Children[next.PID]...)
	}

	switch {
	case e.Caffeinate != 0 && e.FreshChild != nil:
		e.Reason = BusyCaffeinate
	case e.RecentCPU >= CPUThreshold:
		e.Reason = BusyRecentCPU
	case e.TopBackground != nil && e.TopBackground.CPU >= BackgroundCPUThreshold:
		e.Reason = BusyBackground
	}
	return e
}

// parentAgent returns the nearest ancestor that is an agent, or 0.
func parentAgent(agent ProcessSnapshot, tree Tree) int32 {
	pid := agent.PPID
	visited := map[int32]bool{agent.PID: true}
	for len(visited) < maxTreeVisit && !visited[pid] {
		visited[pid] = true
		p, ok := tree.ByPID[pid]
		if !ok {
			return 0
		}
		if _, isAgent := tree.Agents[pid]; isAgent {
			return pid
		}
		pid = p.PPID
	}
	return 0
}
