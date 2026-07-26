// Command gentiles is a one-off asset generator for the grid-sim-game example.
// It bakes the game's four 32x32 tile sprites (a blank grid cell, and the
// generator/house/line tiles, each labelled with its letter) as PNG files
// under the module's assets/art directory, using only the Go standard library
// plus golang.org/x/image's basic bitmap font (no external font/image assets
// needed). Run with:
//
//	go run ./tools/gentiles
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	xdraw "golang.org/x/image/draw"
)

const tileSize = 32

// tileSpec describes one tile sprite to generate.
type tileSpec struct {
	name   string
	bg     color.RGBA
	border color.RGBA
	fg     color.RGBA
	letter string // empty for the blank tile
}

var tiles = []tileSpec{
	{name: "blank", bg: color.RGBA{32, 36, 46, 255}, border: color.RGBA{55, 60, 74, 255}},
	{name: "generator", bg: color.RGBA{224, 150, 40, 255}, border: color.RGBA{150, 96, 20, 255}, fg: color.RGBA{30, 20, 0, 255}, letter: "G"},
	{name: "house", bg: color.RGBA{70, 170, 90, 255}, border: color.RGBA{30, 110, 55, 255}, fg: color.RGBA{10, 40, 20, 255}, letter: "H"},
	{name: "line", bg: color.RGBA{120, 124, 138, 255}, border: color.RGBA{70, 74, 88, 255}, fg: color.RGBA{255, 255, 255, 255}, letter: "L"},
}

func main() {
	outDir := defaultOutDir()
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gentiles:", err)
		os.Exit(1)
	}

	for _, t := range tiles {
		img := renderTile(t)
		path := filepath.Join(outDir, t.name+".png")
		if err := savePNG(path, img); err != nil {
			fmt.Fprintln(os.Stderr, "gentiles:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}

// renderTile draws one tile: a solid background, a 1px border (so adjacent
// tiles read as a grid), and an optional centered, scaled-up letter.
func renderTile(t tileSpec) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(t.bg), image.Point{}, draw.Src)

	for x := 0; x < tileSize; x++ {
		img.Set(x, 0, t.border)
		img.Set(x, tileSize-1, t.border)
	}
	for y := 0; y < tileSize; y++ {
		img.Set(0, y, t.border)
		img.Set(tileSize-1, y, t.border)
	}

	if t.letter != "" {
		drawLetter(img, t.letter, t.fg)
	}
	return img
}

// drawLetter renders a single glyph from the standard library's 7x13 bitmap
// font, scales it up with nearest-neighbor sampling (to stay crisp for pixel
// art), and composites it centered onto dst.
func drawLetter(dst *image.RGBA, letter string, fg color.RGBA) {
	face := basicfont.Face7x13

	glyph := image.NewRGBA(image.Rect(0, 0, face.Width, face.Ascent+face.Descent))
	d := &font.Drawer{
		Dst:  glyph,
		Src:  image.NewUniform(fg),
		Face: face,
		Dot:  fixed.P(0, face.Ascent),
	}
	d.DrawString(letter)

	const scale = 3
	scaled := image.NewRGBA(image.Rect(0, 0, glyph.Bounds().Dx()*scale, glyph.Bounds().Dy()*scale))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), glyph, glyph.Bounds(), xdraw.Over, nil)

	offX := (tileSize - scaled.Bounds().Dx()) / 2
	offY := (tileSize - scaled.Bounds().Dy()) / 2
	dr := image.Rect(offX, offY, offX+scaled.Bounds().Dx(), offY+scaled.Bounds().Dy())
	draw.Draw(dst, dr, scaled, image.Point{}, draw.Over)
}

// defaultOutDir resolves assets/art relative to this source file's location
// (tools/gentiles/main.go), so the output path is correct regardless of the
// working directory `go run` is invoked from.
func defaultOutDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "assets", "art")
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
