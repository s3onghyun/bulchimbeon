// Package sprites draws the private for the Linux tray icon: the frames of every pose in every
// uniform theme (Swift `Sprites` and `UniformTheme`).
//
// The rows and colors are not ported by hand. The Swift tests export them from the core to
// testdata/engine/sprites.json; sprites.json in this directory is a byte-identical copy (the
// tests check it), embedded in the binary.
package sprites

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
)

//go:embed sprites.json
var fixture []byte

// Theme is one uniform theme: its names and its dressed frames.
type Theme struct {
	ID     string
	NameKo string
	NameEn string
	NameZh string

	palette    map[byte]color.RGBA
	frames     [][]string
	poseFrames map[string][]int
}

type sheet struct {
	size     int
	interval time.Duration
	themes   []Theme
}

var load = sync.OnceValue(func() sheet {
	s, err := parse(fixture)
	if err != nil {
		panic("sprites.json: " + err.Error()) // the tests parse the same bytes
	}
	return s
})

func parse(data []byte) (sheet, error) {
	var f struct {
		Size         int      `json:"size"`
		FrameSeconds float64  `json:"frameSeconds"`
		Transparent  string   `json:"transparent"`
		Poses        []string `json:"poses"`
		Themes       []struct {
			ID         string            `json:"id"`
			Name       map[string]string `json:"name"`
			Palette    map[string]string `json:"palette"`
			PoseFrames map[string][]int  `json:"poseFrames"`
			Frames     [][]string        `json:"frames"`
		} `json:"themes"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return sheet{}, err
	}
	if f.Size <= 0 || f.FrameSeconds <= 0 || f.Transparent != "." || len(f.Themes) == 0 {
		return sheet{}, fmt.Errorf("bad header")
	}
	s := sheet{size: f.Size, interval: time.Duration(f.FrameSeconds * float64(time.Second))}
	for _, t := range f.Themes {
		theme := Theme{ID: t.ID, NameKo: t.Name["ko"], NameEn: t.Name["en"], NameZh: t.Name["zh"],
			palette: map[byte]color.RGBA{}, frames: t.Frames, poseFrames: t.PoseFrames}
		for ch, hex := range t.Palette {
			v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
			if len(ch) != 1 || len(hex) != 7 || err != nil {
				return sheet{}, fmt.Errorf("%s: bad color %q: %q", t.ID, ch, hex)
			}
			theme.palette[ch[0]] = color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
		}
		for _, pose := range f.Poses {
			if len(t.PoseFrames[pose]) == 0 {
				return sheet{}, fmt.Errorf("%s: no frames for %s", t.ID, pose)
			}
			for _, i := range t.PoseFrames[pose] {
				if i < 0 || i >= len(t.Frames) {
					return sheet{}, fmt.Errorf("%s/%s: frame %d out of range", t.ID, pose, i)
				}
			}
		}
		for i, rows := range t.Frames {
			if len(rows) != f.Size {
				return sheet{}, fmt.Errorf("%s: frame %d has %d rows", t.ID, i, len(rows))
			}
			for _, row := range rows {
				if len(row) != f.Size {
					return sheet{}, fmt.Errorf("%s: frame %d row %q", t.ID, i, row)
				}
				for j := 0; j < len(row); j++ {
					if _, ok := theme.palette[row[j]]; !ok && row[j] != '.' {
						return sheet{}, fmt.Errorf("%s: frame %d: no color for %q", t.ID, i, row[j])
					}
				}
			}
		}
		s.themes = append(s.themes, theme)
	}
	return s, nil
}

// Size is the sprite's width and height in pixels (16).
func Size() int { return load().size }

// FrameInterval is how long each frame shows, as on the macOS menu bar (0.5 s).
func FrameInterval() time.Duration { return load().interval }

// Themes lists the uniform themes in menu order; the default theme is first.
func Themes() []Theme { return load().themes }

// Valid reports whether id is a registered theme.
func Valid(id string) bool {
	for _, t := range Themes() {
		if t.ID == id {
			return true
		}
	}
	return false
}

// Named returns the theme with this id, or the default theme for an unknown one (Swift
// `UniformTheme.named`).
func Named(id string) Theme {
	for _, t := range Themes() {
		if t.ID == id {
			return t
		}
	}
	return Themes()[0]
}

// Name is the theme's name in the language.
func (t Theme) Name(lang engine.Lang) string { return engine.T(lang, t.NameKo, t.NameEn, t.NameZh) }

// Frames are the rows of each frame of a pose, one frame per FrameInterval. An unknown pose
// shows the sleeping private.
func (t Theme) Frames(pose engine.Pose) [][]string {
	indices, ok := t.poseFrames[string(pose)]
	if !ok {
		indices = t.poseFrames[string(engine.PoseSleeping)]
	}
	frames := make([][]string, len(indices))
	for i, n := range indices {
		frames[i] = t.frames[n]
	}
	return frames
}

// Render draws one frame into a size × size image: each sprite pixel becomes an integer
// square (no smoothing), centered, on a transparent background. Sizes below the sprite's
// size are drawn at the sprite's size.
func (t Theme) Render(rows []string, size int) *image.RGBA {
	size = max(size, Size())
	scale := size / Size()
	offset := (size - scale*Size()) / 2
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y, row := range rows {
		for x := 0; x < len(row); x++ {
			c, ok := t.palette[row[x]]
			if !ok {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(offset+x*scale+dx, offset+y*scale+dy, c)
				}
			}
		}
	}
	return img
}
