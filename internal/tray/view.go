package tray

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/s3onghyun/bulchimbeon/internal/daemon"
	"github.com/s3onghyun/bulchimbeon/internal/dbus"
	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
	"github.com/s3onghyun/bulchimbeon/internal/sprites"
)

// view is what the tray shows at one moment.
type view struct {
	lang     engine.Lang
	pose     engine.Pose
	theme    string
	headline string   // the first line of `bulchimbeon status`: tooltip and menu
	status   []string // the lines below it the menu shows
}

// snapshot is what a view is made from.
type snapshot struct {
	state     *daemon.State
	statePath string
	settings  settings.Settings
	lang      engine.Lang
	now       time.Time
	alive     func(pid int) bool
	home      string
}

// statusKeys are the `bulchimbeon status` lines the menu shows, in their order there.
var statusKeys = map[string]bool{"start": true, "leftovers": true, "lock": true, "why": true, "agents": true, "handover": true}

// maxLabel keeps a menu item to a readable width.
const maxLabel = 100

// buildView takes the status text from the same code as `bulchimbeon status`, and the pose
// from the state file, where the daemon writes the engine's pose: no duty decision is made here.
func buildView(in snapshot) view {
	r := daemon.Status(daemon.StatusInput{State: in.state, StatePath: in.statePath, NoHolders: true,
		Alive: in.alive, Lang: in.lang, Now: in.now, Home: in.home})
	v := view{lang: in.lang, pose: engine.PoseSleeping, theme: sprites.Named(in.settings.UniformTheme).ID, headline: r.Headline}
	if r.Running {
		v.pose = engine.Pose(in.state.Pose) // an unknown pose draws the sleeping private
	}
	for _, l := range r.Lines {
		if statusKeys[l.Key] {
			v.status = append(v.status, statusLabel(l))
		}
	}
	return v
}

// statusLabel joins a label and its text the way a menu reads, "Lock: held". The terminal
// labels have no colon in Korean (they are column headers there).
func statusLabel(l daemon.StatusLine) string {
	text := l.Text
	if label := strings.TrimSpace(l.Label); label != "" {
		if !strings.HasSuffix(label, ":") && !strings.HasSuffix(label, "：") {
			label += ":"
		}
		if !strings.HasSuffix(label, "：") {
			label += " "
		}
		text = label + text
	}
	return truncate(text, maxLabel)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// Menu item ids. Status lines are numbered from 1.
const (
	idRoot      int32 = 0
	idSeparator int32 = 80
	idUniform   int32 = 90
	idQuitSep   int32 = 81
	idQuit      int32 = 99
	idTheme     int32 = 100 // + index in sprites.Themes()
)

// item is one dbusmenu node.
type item struct {
	id       int32
	props    map[string]dbus.Variant
	children []item
}

func str(s string) dbus.Variant { return dbus.MakeVariant("s", s) }

// mnemonic escapes underscores, which dbusmenu labels treat as access-key markers.
func mnemonic(s string) string { return strings.ReplaceAll(s, "_", "__") }

// menu is the tray menu: the status block, the uniform themes and quit.
func (v view) menu() item {
	t := func(ko, en, zh string) string { return engine.T(v.lang, ko, en, zh) }
	root := item{id: idRoot, props: map[string]dbus.Variant{"children-display": str("submenu")}}
	for i, line := range append([]string{v.headline}, v.status...) {
		root.children = append(root.children, item{id: int32(1 + i), props: map[string]dbus.Variant{
			"label": str(mnemonic(line)), "enabled": dbus.MakeVariant("b", false)}})
	}
	root.children = append(root.children, item{id: idSeparator, props: map[string]dbus.Variant{"type": str("separator")}})

	// the same menu title as the macOS app
	uniform := item{id: idUniform, props: map[string]dbus.Variant{
		"label": str(t("🧥 군복", "🧥 Uniform", "🧥 制服")), "children-display": str("submenu")}}
	for i, theme := range sprites.Themes() {
		state := int32(0)
		if theme.ID == v.theme {
			state = 1
		}
		uniform.children = append(uniform.children, item{id: idTheme + int32(i), props: map[string]dbus.Variant{
			"label": str(mnemonic(theme.Name(v.lang))), "toggle-type": str("radio"), "toggle-state": dbus.MakeVariant("i", state)}})
	}
	root.children = append(root.children, uniform,
		item{id: idQuitSep, props: map[string]dbus.Variant{"type": str("separator")}},
		item{id: idQuit, props: map[string]dbus.Variant{
			"label": str(t("트레이 아이콘 닫기 (근무는 계속)", "Quit tray (the watch keeps running)", "关闭托盘图标（值班继续）"))}})
	return root
}

// find returns the node with this id.
func (it item) find(id int32) (item, bool) {
	if it.id == id {
		return it, true
	}
	for _, c := range it.children {
		if found, ok := c.find(id); ok {
			return found, true
		}
	}
	return item{}, false
}

// properties keeps only the named properties; no names means all.
func (it item) properties(names []string) map[string]dbus.Variant {
	if len(names) == 0 {
		return it.props
	}
	out := map[string]dbus.Variant{}
	for _, n := range names {
		if v, ok := it.props[n]; ok {
			out[n] = v
		}
	}
	return out
}

// layout is the (ia{sv}av) of GetLayout, depth levels down (-1: all).
func (it item) layout(depth int32, names []string) []any {
	children := []dbus.Variant{}
	if depth != 0 {
		for _, c := range it.children {
			children = append(children, dbus.MakeVariant("(ia{sv}av)", c.layout(depth-1, names)))
		}
	}
	return []any{it.id, it.properties(names), children}
}

// sameMenu reports whether two views have the same menu.
func sameMenu(a, b view) bool {
	if a.lang != b.lang || a.theme != b.theme || a.headline != b.headline || len(a.status) != len(b.status) {
		return false
	}
	for i := range a.status {
		if a.status[i] != b.status[i] {
			return false
		}
	}
	return true
}
