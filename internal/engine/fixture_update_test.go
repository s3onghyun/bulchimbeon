package engine

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testdata/engine/update.json: version order, the release metadata, the once-a-day schedule and
// what the menu offers.

type releaseJSON struct {
	Version       string  `json:"version"`
	CriticalBelow *string `json:"criticalBelow"`
}

func (r *releaseJSON) release(t *testing.T) *Release {
	t.Helper()
	if r == nil {
		return nil
	}
	v, ok := ParseVersion(r.Version)
	if !ok {
		t.Fatalf("fixture version %q", r.Version)
	}
	rel := &Release{Version: v}
	if r.CriticalBelow != nil {
		below, ok := ParseVersion(*r.CriticalBelow)
		if !ok {
			t.Fatalf("fixture version %q", *r.CriticalBelow)
		}
		rel.CriticalBelow = &below
	}
	return rel
}

type updateFixture struct {
	CheckIntervalSeconds float64 `json:"checkIntervalSeconds"`
	MaxResponseBytes     int     `json:"maxResponseBytes"`
	LatestReleaseAPI     string  `json:"latestReleaseAPI"`
	Versions             []struct {
		Name   string  `json:"name"`
		Input  string  `json:"input"`
		Expect *string `json:"expect"`
	} `json:"versions"`
	Compare []struct {
		A, B   string
		Expect int `json:"expect"`
	} `json:"compare"`
	Releases []struct {
		Name   string          `json:"name"`
		JSON   json.RawMessage `json:"json"`
		Text   *string         `json:"text"`
		Expect *releaseJSON    `json:"expect"`
	} `json:"releases"`
	Due []struct {
		Name    string   `json:"name"`
		Enabled bool     `json:"enabled"`
		Current string   `json:"current"`
		Now     float64  `json:"now"`
		Last    *float64 `json:"last"`
		Expect  bool     `json:"expect"`
	} `json:"due"`
	Offers []struct {
		Name    string       `json:"name"`
		Enabled bool         `json:"enabled"`
		Current string       `json:"current"`
		Latest  *releaseJSON `json:"latest"`
		Expect  *struct {
			Version  string `json:"version"`
			Critical bool   `json:"critical"`
		} `json:"expect"`
	} `json:"offers"`
	Texts []struct {
		Version   string            `json:"version"`
		Critical  bool              `json:"critical"`
		MenuTitle map[string]string `json:"menuTitle"`
		Summary   map[string]string `json:"summary"`
	} `json:"texts"`
	ReleaseNotesTitle map[string]string `json:"releaseNotesTitle"`
	URLs              []struct {
		Version     string `json:"version"`
		ReleasePage string `json:"releasePage"`
		MacZip      string `json:"macZip"`
		MacChecksum string `json:"macChecksum"`
	} `json:"urls"`
	RequestHeaders []struct {
		Current string            `json:"current"`
		Expect  map[string]string `json:"expect"`
	} `json:"requestHeaders"`
}

func loadUpdate(t *testing.T) updateFixture {
	var f updateFixture
	load(t, "update.json", &f)
	return f
}

func mustVersion(t *testing.T, s string) Version {
	t.Helper()
	v, ok := ParseVersion(s)
	if !ok {
		t.Fatalf("ParseVersion(%q) failed", s)
	}
	return v
}

func TestFixtureUpdateConstants(t *testing.T) {
	f := loadUpdate(t)
	if UpdateCheckInterval != time.Duration(f.CheckIntervalSeconds*float64(time.Second)) {
		t.Errorf("UpdateCheckInterval = %v, want %vs", UpdateCheckInterval, f.CheckIntervalSeconds)
	}
	if MaxReleaseResponseBytes != f.MaxResponseBytes {
		t.Errorf("MaxReleaseResponseBytes = %d, want %d", MaxReleaseResponseBytes, f.MaxResponseBytes)
	}
	if LatestReleaseAPI != f.LatestReleaseAPI {
		t.Errorf("LatestReleaseAPI = %q, want %q", LatestReleaseAPI, f.LatestReleaseAPI)
	}
}

func TestFixtureVersions(t *testing.T) {
	f := loadUpdate(t)
	for _, c := range f.Versions {
		v, ok := ParseVersion(c.Input)
		switch {
		case c.Expect == nil && ok:
			t.Errorf("%s: ParseVersion(%q) = %s, want invalid", c.Name, c.Input, v)
		case c.Expect != nil && (!ok || v.String() != *c.Expect):
			t.Errorf("%s: ParseVersion(%q) = %s %v, want %s", c.Name, c.Input, v, ok, *c.Expect)
		}
	}
	for _, c := range f.Compare {
		a, b := mustVersion(t, c.A), mustVersion(t, c.B)
		if got := a.Compare(b); got != c.Expect {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.A, c.B, got, c.Expect)
		}
		if got := b.Compare(a); got != -c.Expect {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.B, c.A, got, -c.Expect)
		}
	}
}

func TestFixtureReleases(t *testing.T) {
	f := loadUpdate(t)
	for _, c := range f.Releases {
		data := []byte(c.JSON)
		if c.Text != nil {
			data = []byte(*c.Text)
		}
		got, ok := ParseRelease(data)
		want := c.Expect.release(t)
		switch {
		case want == nil && ok:
			t.Errorf("%s: ParseRelease = %+v, want none", c.Name, got)
		case want != nil && (!ok || !reflect.DeepEqual(got, *want)):
			t.Errorf("%s: ParseRelease = %+v %v, want %+v", c.Name, got, ok, *want)
		}
	}
	padded := `{"tag_name": "v9.0.0", "body": "` + strings.Repeat("x", MaxReleaseResponseBytes) + `"}`
	if _, ok := ParseRelease([]byte(padded)); ok {
		t.Error("ParseRelease accepted a response over the size limit")
	}
}

func TestFixtureUpdateDue(t *testing.T) {
	f := loadUpdate(t)
	at := func(s float64) time.Time { return time.Unix(0, 0).Add(time.Duration(s * float64(time.Second))) }
	for _, c := range f.Due {
		var last *time.Time
		if c.Last != nil {
			l := at(*c.Last)
			last = &l
		}
		var current *Version
		if v, ok := ParseVersion(c.Current); ok {
			current = &v
		}
		if got := UpdateDue(c.Enabled, current, at(c.Now), last); got != c.Expect {
			t.Errorf("%s: UpdateDue = %v, want %v", c.Name, got, c.Expect)
		}
	}
}

func TestFixtureUpdateOffers(t *testing.T) {
	f := loadUpdate(t)
	for _, c := range f.Offers {
		var current *Version
		if v, ok := ParseVersion(c.Current); ok {
			current = &v
		}
		got, ok := DecideOffer(current, c.Latest.release(t), c.Enabled)
		switch {
		case c.Expect == nil && ok:
			t.Errorf("%s: DecideOffer = %+v, want none", c.Name, got)
		case c.Expect != nil && (!ok || got.Version.String() != c.Expect.Version || got.Critical != c.Expect.Critical):
			t.Errorf("%s: DecideOffer = %+v %v, want %+v", c.Name, got, ok, *c.Expect)
		}
	}
}

func TestFixtureUpdateTexts(t *testing.T) {
	f := loadUpdate(t)
	for _, c := range f.Texts {
		o := Offer{Version: mustVersion(t, c.Version), Critical: c.Critical}
		for _, l := range langs {
			if got := o.MenuTitle(l); got != c.MenuTitle[l.String()] {
				t.Errorf("MenuTitle(%s) = %q, want %q", l, got, c.MenuTitle[l.String()])
			}
			if got := o.Summary(l); got != c.Summary[l.String()] {
				t.Errorf("Summary(%s) = %q, want %q", l, got, c.Summary[l.String()])
			}
		}
	}
	for _, l := range langs {
		if got := ReleaseNotesTitle(l); got != f.ReleaseNotesTitle[l.String()] {
			t.Errorf("ReleaseNotesTitle(%s) = %q, want %q", l, got, f.ReleaseNotesTitle[l.String()])
		}
	}
}

func TestFixtureUpdateURLsAndHeaders(t *testing.T) {
	f := loadUpdate(t)
	for _, c := range f.URLs {
		v := mustVersion(t, c.Version)
		if got := ReleasePageURL(v); got != c.ReleasePage {
			t.Errorf("ReleasePageURL(%s) = %q, want %q", v, got, c.ReleasePage)
		}
		if got := ReleaseDownloadURL(v, "Bulchimbeon-"+v.String()+".zip"); got != c.MacZip {
			t.Errorf("ReleaseDownloadURL(%s) = %q, want %q", v, got, c.MacZip)
		}
	}
	for _, c := range f.RequestHeaders {
		if got := UpdateRequestHeaders(mustVersion(t, c.Current)); !reflect.DeepEqual(got, c.Expect) {
			t.Errorf("UpdateRequestHeaders(%s) = %v, want %v", c.Current, got, c.Expect)
		}
	}
}
