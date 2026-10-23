package sprites

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
)

// The shared fixture, as the Swift tests write it.
var shared = filepath.Join("..", "..", "testdata", "engine", "sprites.json")

// IconSizes are the pixmap sizes the tray offers.
var iconSizes = []int{16, 22, 24, 32, 48}

func TestEmbeddedCopyIsTheSharedFixture(t *testing.T) {
	want, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixture, want) {
		t.Fatal("internal/sprites/sprites.json differs from testdata/engine/sprites.json: cp testdata/engine/sprites.json internal/sprites/")
	}
}

// Every frame of every pose in every theme, at every icon size, is the fixture's rows and
// palette pixel for pixel: integer squares, centered, transparent elsewhere.
func TestRenderMatchesFixtureRows(t *testing.T) {
	data, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Size   int      `json:"size"`
		Poses  []string `json:"poses"`
		Themes []struct {
			ID         string            `json:"id"`
			Palette    map[string]string `json:"palette"`
			PoseFrames map[string][]int  `json:"poseFrames"`
			Frames     [][]string        `json:"frames"`
		} `json:"themes"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Themes) != len(Themes()) {
		t.Fatalf("%d themes, fixture has %d", len(Themes()), len(f.Themes))
	}
	checked := 0
	for ti, ft := range f.Themes {
		theme := Themes()[ti]
		if theme.ID != ft.ID {
			t.Fatalf("theme %d is %q, fixture %q", ti, theme.ID, ft.ID)
		}
		for _, pose := range f.Poses {
			frames := theme.Frames(engine.Pose(pose))
			if len(frames) != len(ft.PoseFrames[pose]) {
				t.Fatalf("%s/%s: %d frames, fixture %d", ft.ID, pose, len(frames), len(ft.PoseFrames[pose]))
			}
			for fi, n := range ft.PoseFrames[pose] {
				rows := ft.Frames[n]
				if !reflect.DeepEqual(frames[fi], rows) {
					t.Fatalf("%s/%s frame %d rows differ", ft.ID, pose, fi)
				}
				for _, size := range iconSizes {
					img := theme.Render(frames[fi], size)
					if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
						t.Fatalf("%s/%s %dpx: bounds %v", ft.ID, pose, size, b)
					}
					scale := size / f.Size
					offset := (size - scale*f.Size) / 2
					for y := 0; y < size; y++ {
						for x := 0; x < size; x++ {
							want := color.RGBA{}
							sx, sy := (x-offset)/scale, (y-offset)/scale
							if x >= offset && y >= offset && sx < f.Size && sy < f.Size {
								if hex, ok := ft.Palette[string(rows[sy][sx])]; ok {
									v, _ := strconv.ParseUint(hex[1:], 16, 32)
									want = color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
								}
							}
							if got := img.RGBAAt(x, y); got != want {
								t.Fatalf("%s/%s frame %d %dpx (%d,%d) = %v, want %v", ft.ID, pose, fi, size, x, y, got, want)
							}
						}
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("nothing checked")
	}
}

func TestThemeRegistry(t *testing.T) {
	if Themes()[0].ID != engine.DefaultUniformTheme {
		t.Errorf("first theme %q, want the default", Themes()[0].ID)
	}
	if Named("navy").ID != "navy" || Named("").ID != engine.DefaultUniformTheme || Named("camo").ID != engine.DefaultUniformTheme {
		t.Error("Named does not fall back to the default")
	}
	if !Valid("arctic") || Valid("Arctic") || Valid("") {
		t.Error("Valid")
	}
	for _, theme := range Themes() {
		for _, lang := range []engine.Lang{engine.KO, engine.EN, engine.ZH} {
			if theme.Name(lang) == "" {
				t.Errorf("%s has no %s name", theme.ID, lang)
			}
		}
	}
	if Named("navy").Name(engine.KO) != "네이비" || Named("navy").Name(engine.EN) != "Navy" {
		t.Errorf("navy names: %q %q", Named("navy").Name(engine.KO), Named("navy").Name(engine.EN))
	}
}

// Every pose the engine can be in has frames; an unknown pose sleeps.
func TestEnginePosesHaveFrames(t *testing.T) {
	for _, pose := range []engine.Pose{engine.PoseSleeping, engine.PoseStandby, engine.PoseDuty, engine.PoseSalute, engine.PoseHeat} {
		if len(Named("default").Frames(pose)) == 0 {
			t.Errorf("no frames for %s", pose)
		}
	}
	if !reflect.DeepEqual(Named("default").Frames("marching"), Named("default").Frames(engine.PoseSleeping)) {
		t.Error("unknown pose does not fall back to sleeping")
	}
}

func TestFrameIntervalAndSize(t *testing.T) {
	if FrameInterval() != 500*time.Millisecond || Size() != 16 {
		t.Errorf("interval %v size %d", FrameInterval(), Size())
	}
}

func TestParseRejectsBrokenSheets(t *testing.T) {
	for name, data := range map[string]string{
		"not json":      `{`,
		"no themes":     `{"size": 16, "frameSeconds": 0.5, "transparent": ".", "poses": [], "themes": []}`,
		"bad color":     `{"size": 1, "frameSeconds": 0.5, "transparent": ".", "poses": ["a"], "themes": [{"id": "x", "palette": {"K": "black"}, "poseFrames": {"a": [0]}, "frames": [["K"]]}]}`,
		"unknown char":  `{"size": 1, "frameSeconds": 0.5, "transparent": ".", "poses": ["a"], "themes": [{"id": "x", "palette": {}, "poseFrames": {"a": [0]}, "frames": [["K"]]}]}`,
		"missing pose":  `{"size": 1, "frameSeconds": 0.5, "transparent": ".", "poses": ["a"], "themes": [{"id": "x", "palette": {}, "poseFrames": {}, "frames": [["."]]}]}`,
		"frame range":   `{"size": 1, "frameSeconds": 0.5, "transparent": ".", "poses": ["a"], "themes": [{"id": "x", "palette": {}, "poseFrames": {"a": [3]}, "frames": [["."]]}]}`,
		"short row":     `{"size": 2, "frameSeconds": 0.5, "transparent": ".", "poses": ["a"], "themes": [{"id": "x", "palette": {}, "poseFrames": {"a": [0]}, "frames": [["..", "."]]}]}`,
		"transparent ?": `{"size": 1, "frameSeconds": 0.5, "transparent": " ", "poses": ["a"], "themes": [{"id": "x", "palette": {}, "poseFrames": {"a": [0]}, "frames": [["."]]}]}`,
	} {
		if _, err := parse([]byte(data)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
