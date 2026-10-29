// Package update is the Linux daemon's once-a-day check for a newer release. It only asks
// GitHub for the latest release (one HTTPS GET with the headers engine.UpdateRequestHeaders
// gives: no settings, logs or IDs), remembers the answer in update.json and never installs
// anything: `status`, `doctor` and the tray show the package-manager command instead.
//
// What to ask, when and what to offer is decided by internal/engine (the shared fixture
// testdata/engine/update.json); this package carries it out.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
)

// How this binary was installed (internal/platform/linux detects it from files).
const (
	InstallDeb      = "deb"
	InstallRPM      = "rpm"
	InstallAUR      = "aur"
	InstallHomebrew = "homebrew"
	InstallManual   = "manual" // the tarball and install-linux.sh, or unknown
)

// Timeout bounds one check.
const Timeout = 15 * time.Second

// Fetcher gets one URL with the given headers and returns the body.
type Fetcher func(ctx context.Context, url string, headers map[string]string) ([]byte, error)

// HTTPFetcher is the real Fetcher: HTTPS only, no cookies, the body capped at
// engine.MaxReleaseResponseBytes.
func HTTPFetcher(timeout time.Duration) Fetcher { return newFetcher(&http.Client{Timeout: timeout}) }

func newFetcher(client *http.Client) Fetcher {
	c := *client
	c.Jar = nil
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 5 {
			return errors.New("refusing redirect to " + req.URL.Scheme + "://" + req.URL.Host)
		}
		return nil
	}
	return func(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
		if !strings.HasPrefix(url, "https://") {
			return nil, errors.New("not an https URL")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, engine.MaxReleaseResponseBytes+1))
		if err != nil {
			return nil, err
		}
		if len(body) > engine.MaxReleaseResponseBytes {
			return nil, errors.New("response too large")
		}
		return body, nil
	}
}

// Record is update.json: when the last check was made and what is known. Not part of the
// compatibility promise.
type Record struct {
	CheckedAt     time.Time `json:"checkedAt"`
	Latest        string    `json:"latest,omitempty"`
	CriticalBelow string    `json:"criticalBelow,omitempty"`
	Error         string    `json:"error,omitempty"` // why the last check failed
}

// Release is the latest release the record knows, if any.
func (r Record) Release() *engine.Release {
	v, ok := engine.ParseVersion(r.Latest)
	if !ok {
		return nil
	}
	rel := &engine.Release{Version: v}
	if below, ok := engine.ParseVersion(r.CriticalBelow); ok {
		rel.CriticalBelow = &below
	}
	return rel
}

func (r Record) lastCheck() *time.Time {
	if r.CheckedAt.IsZero() {
		return nil
	}
	t := r.CheckedAt
	return &t
}

// Load reads update.json; a missing or broken file is an empty record.
func Load(path string) Record {
	var r Record
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &r) != nil {
		r = Record{}
	}
	return r
}

// Save writes update.json (0600, through a temporary file).
func Save(path string, r Record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return settings.WriteFileAtomic(path, append(data, '\n'))
}

// Checker runs the check for the daemon without ever blocking its tick.
type Checker struct {
	path    string
	current *engine.Version
	fetch   Fetcher

	mu      sync.Mutex
	loaded  bool
	record  Record
	running chan struct{} // closed when the running check ends
}

// NewChecker checks for releases newer than version (the build's own; "dev" never checks),
// keeping its record at path.
func NewChecker(path, version string, fetch Fetcher) *Checker {
	c := &Checker{path: path, fetch: fetch}
	if v, ok := engine.ParseVersion(version); ok {
		c.current = &v
	}
	return c
}

// Poll starts a check in the background when one is due and returns the offer known now.
func (c *Checker) Poll(enabled bool, now time.Time) (engine.Offer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded {
		c.record, c.loaded = Load(c.path), true
	}
	if c.running == nil && engine.UpdateDue(enabled, c.current, now, c.record.lastCheck()) {
		done := make(chan struct{})
		c.running = done
		go c.check(now, done)
	}
	return engine.DecideOffer(c.current, c.record.Release(), enabled)
}

// Wait waits for a running check (tests).
func (c *Checker) Wait() {
	c.mu.Lock()
	done := c.running
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}

// check asks once. The attempt counts as the day's check whatever the outcome; a failure
// keeps what was known.
func (c *Checker) check(now time.Time, done chan struct{}) {
	defer close(done)
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	body, err := c.fetch(ctx, engine.LatestReleaseAPI, engine.UpdateRequestHeaders(*c.current))

	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = nil
	rec := Record{CheckedAt: now, Latest: c.record.Latest, CriticalBelow: c.record.CriticalBelow}
	if err == nil {
		if r, ok := engine.ParseRelease(body); ok {
			rec.Latest, rec.CriticalBelow = r.Version.String(), ""
			if r.CriticalBelow != nil {
				rec.CriticalBelow = r.CriticalBelow.String()
			}
		} else {
			err = errors.New("the answer was not a release")
		}
	}
	if err != nil {
		rec.Error = err.Error()
		if r := []rune(rec.Error); len(r) > 200 {
			rec.Error = string(r[:200])
		}
	}
	c.record = rec
	if err := Save(c.path, rec); err != nil {
		c.record.Error = err.Error()
	}
}

// Instructions say how to update for the install method: one command or the release page,
// then the release-notes link. Each line fits a tray menu item.
func Instructions(offer engine.Offer, install, arch string, lang engine.Lang) []string {
	t := func(ko, en, zh string) string { return engine.T(lang, ko, en, zh) }
	v := offer.Version.String()
	run := t("실행: ", "Run: ", "运行：")
	fromPage := t(" (릴리스 페이지에서 받은 파일)", " (download it from the release page)", "（从发行页面下载该文件）")
	rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch]
	var how string
	switch {
	case install == InstallDeb && rpmArch != "":
		how = run + "sudo apt install ./bulchimbeon_" + v + "_" + arch + ".deb" + fromPage
	case install == InstallRPM && rpmArch != "":
		how = run + "sudo dnf install ./bulchimbeon-" + v + "-1." + rpmArch + ".rpm" + fromPage
	case install == InstallAUR:
		how = run + "yay -Syu bulchimbeon-bin" + t(" (다른 AUR 도우미도 됨)", " (or your AUR helper)", "（或其他 AUR 助手）")
	case install == InstallHomebrew:
		how = run + "brew update && brew upgrade bulchimbeon-linux"
	default:
		how = t("릴리스 페이지에서 tarball 을 받아 풀고 ./install-linux.sh 를 실행하세요",
			"Download the tarball from the release page, unpack it and run ./install-linux.sh",
			"从发行页面下载 tarball，解压后运行 ./install-linux.sh")
	}
	return []string{how, engine.ReleaseNotesTitle(lang) + t(": ", ": ", "：") + engine.ReleasePageURL(offer.Version)}
}

// DoctorLines describe the update check for `bulchimbeon doctor`.
func DoctorLines(rec Record, enabled bool, version, install, arch string, lang engine.Lang) []string {
	t := func(ko, en, zh string) string { return engine.T(lang, ko, en, zh) }
	if !enabled {
		return []string{t("업데이트 확인: 꺼짐 (켜기: bulchimbeon config set checkForUpdates true)",
			"update check: off (turn it on: bulchimbeon config set checkForUpdates true)",
			"检查更新：已关闭（开启：bulchimbeon config set checkForUpdates true）")}
	}
	current, ok := engine.ParseVersion(version)
	if !ok {
		return []string{t("업데이트 확인: 이 빌드는 하지 않음 (", "update check: not for this build (", "检查更新：此构建不检查（") + version + t(")", ")", "）")}
	}
	last := t("아직 확인 전", "not checked yet", "尚未检查")
	if !rec.CheckedAt.IsZero() {
		last = t("마지막 확인 ", "last checked ", "上次检查 ") + rec.CheckedAt.Local().Format("2006-01-02 15:04")
	}
	lines := []string{t("업데이트 확인: 켜짐, 하루 한 번 api.github.com 에 최신 릴리스만 묻습니다 (끄기: bulchimbeon config set checkForUpdates false) · ",
		"update check: on, asks api.github.com for the latest release once a day (turn it off: bulchimbeon config set checkForUpdates false) · ",
		"检查更新：已开启，每天向 api.github.com 查询一次最新版本（关闭：bulchimbeon config set checkForUpdates false）· ") + last}
	if rec.Error != "" {
		lines = append(lines, t("마지막 확인 실패: ", "last check failed: ", "上次检查失败：")+rec.Error)
	}
	if offer, ok := engine.DecideOffer(&current, rec.Release(), true); ok {
		lines = append(lines, offer.Summary(lang))
		return append(lines, Instructions(offer, install, arch, lang)...)
	}
	if rec.Release() != nil {
		lines = append(lines, t("최신 버전입니다 (", "up to date (", "已是最新版本（")+current.String()+t(")", ")", "）"))
	}
	return lines
}
