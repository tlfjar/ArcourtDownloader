// Icon is an original, code-native courthouse/download mark; no external artwork.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	im := image.NewRGBA(image.Rect(0, 0, 256, 256))
	navy, white, teal := color.RGBA{23, 47, 70, 255}, color.RGBA{244, 246, 250, 255}, color.RGBA{123, 203, 194, 255}
	draw.Draw(im, im.Bounds(), &image.Uniform{navy}, image.Point{}, draw.Src)
	fill := func(x, y, w, h int, c color.RGBA) {
		draw.Draw(im, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	for y := 40; y < 90; y++ {
		half := (y - 40) * 2
		fill(128-half, y, half*2+1, 1, white)
	}
	fill(36, 94, 184, 12, white)
	for _, x := range []int{51, 100, 149, 198} {
		fill(x, 112, 12, 78, white)
	}
	fill(36, 194, 184, 14, white)
	fill(177, 150, 18, 60, teal)
	for y := 198; y < 226; y++ {
		half := 226 - y
		fill(186-half, y, half*2+1, 1, teal)
	}
	f, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, im); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
