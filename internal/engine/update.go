package engine

import (
	"encoding/json"
	"strings"
	"time"
)

// Update check (Swift `AppVersion`, `ReleaseInfo`, `UpdateCheck`, `UpdateOffer`, `ReleaseURLs`),
// pinned by testdata/engine/update.json. The engine only decides; the request itself is made
// by internal/update.

const (
	// UpdateCheckInterval: at most one request a day, whether it succeeded or not.
	UpdateCheckInterval = 24 * time.Hour
	// MaxReleaseResponseBytes: a larger response is not read.
	MaxReleaseResponseBytes = 1 << 20
	LatestReleaseAPI        = "https://api.github.com/repos/s3onghyun/bulchimbeon/releases/latest"
	ReleaseRepository       = "https://github.com/s3onghyun/bulchimbeon"
	LatestReleasePage       = ReleaseRepository + "/releases/latest"
)

// Version is a release version in SemVer 2.0 order. Build metadata is dropped, and a
// git-describe suffix (0.10.2-3-gabc1234) compares as its tag.
type Version struct {
	Major, Minor, Patch int
	Prerelease          []string
}

// ParseVersion reads "0.10.2", "v1.0.0-rc.1", "1.0.0+build" or a git-describe build. Untagged
// builds ("dev", "0.0.0~ci") are not versions.
func ParseVersion(text string) (Version, bool) {
	s := strings.TrimSpace(text)
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		s = s[1:]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		for _, id := range strings.Split(s[i+1:], ".") {
			if !isIdentifier(id) {
				return Version{}, false
			}
		}
		s = s[:i]
	}
	s = dropGitDescribe(s)

	core, pre := s, []string(nil)
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		for _, id := range strings.Split(s[i+1:], ".") {
			if !isIdentifier(id) || hasLeadingZero(id) {
				return Version{}, false
			}
			pre = append(pre, id)
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var n [3]int
	for i, p := range parts {
		if len(p) < 1 || len(p) > 9 || !isNumeric(p) || hasLeadingZero(p) {
			return Version{}, false
		}
		for _, c := range p {
			n[i] = n[i]*10 + int(c-'0')
		}
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2], Prerelease: pre}, true
}

func (v Version) String() string {
	s := itoa(v.Major) + "." + itoa(v.Minor) + "." + itoa(v.Patch)
	if len(v.Prerelease) > 0 {
		s += "-" + strings.Join(v.Prerelease, ".")
	}
	return s
}

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for _, d := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if d[0] != d[1] {
			return sign(d[0] < d[1])
		}
	}
	// no prerelease sorts after any prerelease
	switch {
	case len(v.Prerelease) == 0 && len(o.Prerelease) == 0:
		return 0
	case len(v.Prerelease) == 0:
		return 1
	case len(o.Prerelease) == 0:
		return -1
	}
	for i := 0; i < len(v.Prerelease) && i < len(o.Prerelease); i++ {
		x, y := v.Prerelease[i], o.Prerelease[i]
		if x == y {
			continue
		}
		xn, yn := isNumeric(x), isNumeric(y)
		switch {
		case xn && yn:
			if len(x) != len(y) { // no leading zeros, so length then digits
				return sign(len(x) < len(y))
			}
			return sign(x < y)
		case xn:
			return -1 // numeric < alphanumeric
		case yn:
			return 1
		}
		return sign(x < y)
	}
	if len(v.Prerelease) == len(o.Prerelease) {
		return 0
	}
	return sign(len(v.Prerelease) < len(o.Prerelease))
}

func sign(less bool) int {
	if less {
		return -1
	}
	return 1
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if c != '-' && !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func hasLeadingZero(s string) bool { return len(s) > 1 && s[0] == '0' && isNumeric(s) }

// dropGitDescribe turns "<tag>-<n>-g<hash>[-dirty]" into "<tag>".
func dropGitDescribe(s string) string {
	parts := strings.Split(s, "-")
	if parts[len(parts)-1] == "dirty" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) < 3 {
		return s
	}
	hash := parts[len(parts)-1]
	if len(hash) < 5 || hash[0] != 'g' || !isNumeric(parts[len(parts)-2]) {
		return s
	}
	for _, c := range []byte(hash[1:]) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return s
		}
	}
	return strings.Join(parts[:len(parts)-2], "-")
}

// Release is what the latest GitHub release says: its version and, from a
// `<!-- bulchimbeon: critical [below X.Y.Z] -->` line in its body, which older versions it
// urges to update.
type Release struct {
	Version       Version
	CriticalBelow *Version
}

// ParseRelease reads a `GET /repos/{owner}/{repo}/releases/latest` response. Drafts,
// prereleases, oversized responses and tags that are not versions are ignored.
func ParseRelease(data []byte) (Release, bool) {
	if len(data) > MaxReleaseResponseBytes {
		return Release{}, false
	}
	var p struct {
		TagName    *string `json:"tag_name"`
		Body       *string `json:"body"`
		Draft      *bool   `json:"draft"`
		Prerelease *bool   `json:"prerelease"`
	}
	if json.Unmarshal(data, &p) != nil || p.TagName == nil || (p.Draft != nil && *p.Draft) || (p.Prerelease != nil && *p.Prerelease) {
		return Release{}, false
	}
	v, ok := ParseVersion(*p.TagName)
	if !ok {
		return Release{}, false
	}
	body := ""
	if p.Body != nil {
		body = *p.Body
	}
	return Release{Version: v, CriticalBelow: criticalBelow(body, v)}, true
}

func criticalBelow(body string, release Version) *Version {
	var widest *Version
	for _, line := range strings.Split(body, "\n") {
		text := strings.TrimSpace(line)
		if len([]rune(text)) < 7 || !strings.HasPrefix(text, "<!--") || !strings.HasSuffix(text, "-->") {
			continue
		}
		tokens := strings.Fields(strings.ReplaceAll(text[4:len(text)-3], ":", " : "))
		if len(tokens) < 3 || strings.ToLower(tokens[0]) != "bulchimbeon" || tokens[1] != ":" || strings.ToLower(tokens[2]) != "critical" {
			continue
		}
		var bound *Version
		switch {
		case len(tokens) == 3:
			b := release
			bound = &b
		case len(tokens) == 5 && strings.ToLower(tokens[3]) == "below":
			if b, ok := ParseVersion(tokens[4]); ok {
				bound = &b
			}
		}
		if bound != nil && (widest == nil || widest.Compare(*bound) < 0) {
			widest = bound
		}
	}
	return widest
}

// UpdateDue reports whether a check may be made now: never when it is off or the build has
// no version, and again when the clock went backwards.
func UpdateDue(enabled bool, current *Version, now time.Time, last *time.Time) bool {
	if !enabled || current == nil {
		return false
	}
	return last == nil || now.Before(*last) || now.Sub(*last) >= UpdateCheckInterval
}

// UpdateRequestHeaders are the only headers sent. The language is a fixed "en", so the
// user's locale is not revealed.
func UpdateRequestHeaders(current Version) map[string]string {
	return map[string]string{"User-Agent": "Bulchimbeon/" + current.String(), "Accept": "application/vnd.github+json", "Accept-Language": "en"}
}

// ReleasePageURL is built from the checked version, never taken from the API response.
func ReleasePageURL(v Version) string { return ReleaseRepository + "/releases/tag/v" + v.String() }

// ReleaseDownloadURL is a release file's download address.
func ReleaseDownloadURL(v Version, file string) string {
	return ReleaseRepository + "/releases/download/v" + v.String() + "/" + file
}

// Offer is the update the menu (and `status`) shows.
type Offer struct {
	Version  Version
	Critical bool // recommended: important fix
}

// DecideOffer offers a newer release only while the check is on. It never offers a downgrade.
func DecideOffer(current *Version, latest *Release, enabled bool) (Offer, bool) {
	if !enabled || current == nil || latest == nil || current.Compare(latest.Version) >= 0 {
		return Offer{}, false
	}
	return Offer{Version: latest.Version, Critical: latest.CriticalBelow != nil && current.Compare(*latest.CriticalBelow) < 0}, true
}

// MenuTitle is the menu item, "새 버전 X.Y.Z — 업데이트".
func (o Offer) MenuTitle(lang Lang) string {
	v := o.Version.String()
	return T(lang, "새 버전 "+v+" — 업데이트", "New version "+v+" — Update", "新版本 "+v+" — 更新") + o.criticalSuffix(lang)
}

// Summary is the short form used inside a sentence (`status`): "새 버전 X.Y.Z".
func (o Offer) Summary(lang Lang) string {
	v := o.Version.String()
	return T(lang, "새 버전 "+v, "new version "+v, "新版本 "+v) + o.criticalSuffix(lang)
}

func (o Offer) criticalSuffix(lang Lang) string {
	if !o.Critical {
		return ""
	}
	return T(lang, " (권장: 중요 수정)", " (recommended: important fix)", "（推荐：重要修复）")
}

// ReleaseNotesTitle names the release-notes link.
func ReleaseNotesTitle(lang Lang) string {
	return T(lang, "릴리스 노트 보기", "Release notes", "查看发行说明")
}
