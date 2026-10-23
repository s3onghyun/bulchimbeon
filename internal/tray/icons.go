package tray

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image/png"
	"os"
	"path/filepath"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/settings"
	"github.com/s3onghyun/bulchimbeon/internal/sprites"
)

// Hosts scale an IconPixmap to their icon size with smoothing (XFCE takes the largest one and
// shrinks it), which blurs pixel art. A named icon in an icon theme directory is looked up at
// the exact size instead, so every frame is also written as PNGs in the hicolor layout under
// $XDG_RUNTIME_DIR and announced through IconThemePath and IconName. The pixmaps stay for
// hosts without icon themes.

// iconDirs are the hicolor directories written: the size of the sprite box, and @2 for
// doubled (HiDPI) screens.
var iconDirs = []struct{ size, scale int }{{16, 1}, {22, 1}, {24, 1}, {32, 1}, {48, 1}, {64, 1}, {16, 2}, {22, 2}, {24, 2}}

// enginePoses are the poses the Linux engine shows (patrol is macOS only).
var enginePoses = []engine.Pose{engine.PoseSleeping, engine.PoseStandby, engine.PoseDuty, engine.PoseSalute, engine.PoseHeat}

// iconName names one frame of one theme by its pixels, so a changed sprite or color never
// reuses an old file.
func iconName(theme sprites.Theme, rows []string) string {
	h := fnv.New64a()
	h.Write(theme.Render(rows, sprites.Size()).Pix)
	return fmt.Sprintf("bulchimbeon-%s-%016x", theme.ID, h.Sum64())
}

// writeIcons writes every frame of every theme under root (created 0700), the IconThemePath,
// before the icon is shown, so a host's icon theme finds them all. Files are written through a
// temporary file and rename, so a host never reads half a PNG; an existing file has the same
// pixels (its name says so) and is kept.
func writeIcons(root string) error {
	for _, theme := range sprites.Themes() {
		for _, pose := range enginePoses {
			for _, rows := range theme.Frames(pose) {
				name := iconName(theme, rows)
				for _, d := range iconDirs {
					dir := fmt.Sprintf("%dx%d", d.size, d.size)
					if d.scale > 1 {
						dir += fmt.Sprintf("@%d", d.scale)
					}
					path := filepath.Join(root, "hicolor", dir, "apps", name+".png")
					if _, err := os.Stat(path); err == nil {
						continue
					}
					var buf bytes.Buffer
					if err := png.Encode(&buf, theme.Render(rows, d.size*d.scale)); err != nil {
						return err
					}
					if err := settings.WriteFileAtomic(path, buf.Bytes()); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
