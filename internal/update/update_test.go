package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
)

const latest011 = `{"tag_name": "v0.11.0", "draft": false, "prerelease": false, "body": "<!-- bulchimbeon: critical below 0.10.2 -->\nnotes"}`

// fakeFetcher records each request and answers from a script.
type fakeFetcher struct {
	mu       sync.Mutex
	calls    []string
	headers  []map[string]string
	response string
	err      error
}

func (f *fakeFetcher) fetch(_ context.Context, url string, headers map[string]string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, url)
	f.headers = append(f.headers, headers)
	return []byte(f.response), f.err
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

var t0 = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

func TestCheckerAsksOnceADayAndRemembers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "update.json")
	f := &fakeFetcher{response: latest011}
	c := NewChecker(path, "0.10.1", f.fetch)

	// the first poll starts a check in the background and knows nothing yet
	if _, ok := c.Poll(true, t0); ok {
		t.Error("offer before the check finished")
	}
	c.Wait()
	offer, ok := c.Poll(true, t0.Add(5*time.Second))
	if !ok || offer.Version.String() != "0.11.0" || !offer.Critical {
		t.Fatalf("offer = %+v %v", offer, ok)
	}
	if f.count() != 1 || f.calls[0] != engine.LatestReleaseAPI {
		t.Errorf("calls = %v", f.calls)
	}
	if want := map[string]string{"User-Agent": "Bulchimbeon/0.10.1", "Accept": "application/vnd.github+json", "Accept-Language": "en"}; !reflect.DeepEqual(f.headers[0], want) {
		t.Errorf("headers = %v", f.headers[0])
	}

	// within the day: no new request, even from a restarted daemon
	for _, at := range []time.Duration{time.Hour, 23 * time.Hour} {
		c.Poll(true, t0.Add(at))
		c.Wait()
	}
	restarted := NewChecker(path, "0.10.1", f.fetch)
	if offer, ok := restarted.Poll(true, t0.Add(2*time.Hour)); !ok || offer.Version.String() != "0.11.0" {
		t.Errorf("restarted offer = %+v %v", offer, ok)
	}
	restarted.Wait()
	if f.count() != 1 {
		t.Errorf("asked %d times within a day", f.count())
	}

	// a day later: asked again
	c.Poll(true, t0.Add(24*time.Hour))
	c.Wait()
	if f.count() != 2 {
		t.Errorf("asked %d times after a day", f.count())
	}

	rec := Load(path)
	if rec.Latest != "0.11.0" || rec.CriticalBelow != "0.10.2" || !rec.CheckedAt.Equal(t0.Add(24*time.Hour)) || rec.Error != "" {
		t.Errorf("record = %+v", rec)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("update.json mode = %v %v", info, err)
	}
}

func TestCheckerOffHidesAndNeverAsks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	if err := Save(path, Record{CheckedAt: t0.Add(-48 * time.Hour), Latest: "0.11.0"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeFetcher{response: latest011}
	c := NewChecker(path, "0.10.1", f.fetch)
	if _, ok := c.Poll(false, t0); ok {
		t.Error("offer while the check is off")
	}
	c.Wait()
	if f.count() != 0 {
		t.Errorf("asked %d times while off", f.count())
	}
}

func TestCheckerUntaggedBuildNeverAsks(t *testing.T) {
	f := &fakeFetcher{response: latest011}
	c := NewChecker(filepath.Join(t.TempDir(), "update.json"), "dev", f.fetch)
	c.Poll(true, t0)
	c.Wait()
	if f.count() != 0 {
		t.Errorf("a dev build asked %d times", f.count())
	}
}

func TestCheckerFailureCountsAsTheDaysCheckAndKeepsWhatWasKnown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	if err := Save(path, Record{CheckedAt: t0.Add(-48 * time.Hour), Latest: "0.10.2"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeFetcher{err: errors.New("dial tcp: lookup api.github.com: no such host")}
	c := NewChecker(path, "0.10.1", f.fetch)
	c.Poll(true, t0)
	c.Wait()
	if offer, ok := c.Poll(true, t0.Add(time.Minute)); !ok || offer.Version.String() != "0.10.2" {
		t.Errorf("offer after a failure = %+v %v", offer, ok)
	}
	c.Wait()
	if f.count() != 1 {
		t.Errorf("asked %d times", f.count())
	}
	rec := Load(path)
	if rec.Latest != "0.10.2" || !strings.Contains(rec.Error, "no such host") || !rec.CheckedAt.Equal(t0) {
		t.Errorf("record = %+v", rec)
	}

	// an answer that is not a release is a failure too
	f2 := &fakeFetcher{response: "<html>rate limited</html>"}
	c2 := NewChecker(path, "0.10.1", f2.fetch)
	c2.Poll(true, t0.Add(25*time.Hour))
	c2.Wait()
	if rec := Load(path); rec.Latest != "0.10.2" || rec.Error == "" {
		t.Errorf("record = %+v", rec)
	}
}

func TestLoadToleratesMissingAndBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	if rec := Load(filepath.Join(dir, "missing.json")); !reflect.DeepEqual(rec, Record{}) {
		t.Errorf("missing = %+v", rec)
	}
	broken := filepath.Join(dir, "broken.json")
	os.WriteFile(broken, []byte("{not json"), 0o600)
	if rec := Load(broken); !reflect.DeepEqual(rec, Record{}) {
		t.Errorf("broken = %+v", rec)
	}
	odd := filepath.Join(dir, "odd.json")
	os.WriteFile(odd, []byte(`{"checkedAt": "2026-10-29T09:00:00Z", "latest": "banana", "criticalBelow": "1.0.0"}`), 0o600)
	if r := Load(odd).Release(); r != nil {
		t.Errorf("release from a bad version = %+v", r)
	}
}

func TestHTTPFetcher(t *testing.T) {
	var got http.Header
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		switch r.URL.Path {
		case "/big":
			w.Write([]byte(strings.Repeat("x", engine.MaxReleaseResponseBytes+1)))
		case "/missing":
			http.NotFound(w, r)
		default:
			w.Write([]byte(latest011))
		}
	}))
	defer srv.Close()
	fetch := newFetcher(srv.Client())
	body, err := fetch(context.Background(), srv.URL+"/latest", engine.UpdateRequestHeaders(engine.Version{Major: 0, Minor: 10, Patch: 2}))
	if err != nil || string(body) != latest011 {
		t.Fatalf("fetch = %q %v", body, err)
	}
	if got.Get("User-Agent") != "Bulchimbeon/0.10.2" || got.Get("Accept-Language") != "en" || got.Get("Cookie") != "" || got.Get("Authorization") != "" {
		t.Errorf("headers = %v", got)
	}
	if _, err := fetch(context.Background(), srv.URL+"/big", nil); err == nil {
		t.Error("an oversized answer was read")
	}
	if _, err := fetch(context.Background(), srv.URL+"/missing", nil); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404 = %v", err)
	}
	// only HTTPS
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(latest011)) }))
	defer plain.Close()
	if _, err := fetch(context.Background(), plain.URL, nil); err == nil {
		t.Error("fetched over plain HTTP")
	}
	if _, err := HTTPFetcher(time.Second)(context.Background(), plain.URL, nil); err == nil {
		t.Error("fetched over plain HTTP")
	}
}

func TestInstructions(t *testing.T) {
	v, _ := engine.ParseVersion("0.11.0")
	offer := engine.Offer{Version: v, Critical: true}
	notes := "Release notes: https://github.com/s3onghyun/bulchimbeon/releases/tag/v0.11.0"
	for _, c := range []struct {
		install, arch string
		want          []string
	}{
		{InstallDeb, "amd64", []string{"Run: sudo apt install ./bulchimbeon_0.11.0_amd64.deb (download it from the release page)", notes}},
		{InstallDeb, "arm64", []string{"Run: sudo apt install ./bulchimbeon_0.11.0_arm64.deb (download it from the release page)", notes}},
		{InstallRPM, "amd64", []string{"Run: sudo dnf install ./bulchimbeon-0.11.0-1.x86_64.rpm (download it from the release page)", notes}},
		{InstallRPM, "arm64", []string{"Run: sudo dnf install ./bulchimbeon-0.11.0-1.aarch64.rpm (download it from the release page)", notes}},
		{InstallAUR, "amd64", []string{"Run: yay -Syu bulchimbeon-bin (or your AUR helper)", notes}},
		{InstallHomebrew, "arm64", []string{"Run: brew update && brew upgrade bulchimbeon-linux", notes}},
		{InstallManual, "amd64", []string{"Download the tarball from the release page, unpack it and run ./install-linux.sh", notes}},
		// no package for this architecture: the release page
		{InstallDeb, "riscv64", []string{"Download the tarball from the release page, unpack it and run ./install-linux.sh", notes}},
		{"", "amd64", []string{"Download the tarball from the release page, unpack it and run ./install-linux.sh", notes}},
	} {
		if got := Instructions(offer, c.install, c.arch, engine.EN); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Instructions(%s, %s) = %q, want %q", c.install, c.arch, got, c.want)
		}
	}
	// every line fits a tray menu item, in every language
	for _, lang := range []engine.Lang{engine.KO, engine.EN, engine.ZH} {
		for _, install := range []string{InstallDeb, InstallRPM, InstallAUR, InstallHomebrew, InstallManual} {
			for _, line := range Instructions(offer, install, "arm64", lang) {
				if n := len([]rune(line)); n > 100 {
					t.Errorf("%s %s line is %d long: %q", lang, install, n, line)
				}
			}
		}
	}
}

func TestDoctorLines(t *testing.T) {
	checked := Record{CheckedAt: t0, Latest: "0.11.0", CriticalBelow: "0.10.2"}
	joined := func(lines []string) string { return strings.Join(lines, "\n") }

	got := joined(DoctorLines(checked, true, "0.10.1", InstallAUR, "amd64", engine.EN))
	for _, want := range []string{"update check: on", "bulchimbeon config set checkForUpdates false", "new version 0.11.0 (recommended: important fix)",
		"Run: yay -Syu bulchimbeon-bin", "Release notes: https://github.com/s3onghyun/bulchimbeon/releases/tag/v0.11.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor lacks %q:\n%s", want, got)
		}
	}
	if got := joined(DoctorLines(checked, true, "0.11.0", InstallAUR, "amd64", engine.EN)); !strings.Contains(got, "up to date (0.11.0)") {
		t.Errorf("up to date:\n%s", got)
	}
	if got := joined(DoctorLines(Record{}, true, "0.11.0", InstallAUR, "amd64", engine.EN)); !strings.Contains(got, "not checked yet") {
		t.Errorf("never checked:\n%s", got)
	}
	failed := Record{CheckedAt: t0, Error: "dial tcp: no such host"}
	if got := joined(DoctorLines(failed, true, "0.11.0", InstallAUR, "amd64", engine.EN)); !strings.Contains(got, "last check failed: dial tcp: no such host") {
		t.Errorf("failed:\n%s", got)
	}
	off := joined(DoctorLines(checked, false, "0.10.1", InstallAUR, "amd64", engine.EN))
	if !strings.Contains(off, "update check: off") || !strings.Contains(off, "checkForUpdates true") || strings.Contains(off, "0.11.0") {
		t.Errorf("off:\n%s", off)
	}
	if got := joined(DoctorLines(checked, true, "dev", InstallAUR, "amd64", engine.EN)); !strings.Contains(got, "not for this build (dev)") {
		t.Errorf("dev:\n%s", got)
	}
	if got := joined(DoctorLines(checked, true, "0.10.1", InstallAUR, "amd64", engine.KO)); !strings.Contains(got, "새 버전 0.11.0 (권장: 중요 수정)") {
		t.Errorf("ko:\n%s", got)
	}
}
