package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
)

// appIcon is initialized in init() as a PNG so it renders correctly in the Windows system tray.
var appIcon fyne.Resource

func init() {
	appIcon = fyne.NewStaticResource("icon.png", makeIconPNG())
}

// makeIconPNG generates a 32×32 PNG: blue circle + white |→| (port-forward symbol).
func makeIconPNG() []byte {
	const sz = 32
	img := image.NewNRGBA(image.Rect(0, 0, sz, sz))

	blue := color.NRGBA{R: 21, G: 101, B: 192, A: 255} // #1565C0
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	// Blue circle background
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			dx := float64(x) + 0.5 - float64(sz)/2
			dy := float64(y) + 0.5 - float64(sz)/2
			if math.Sqrt(dx*dx+dy*dy) < float64(sz)/2-0.5 {
				img.SetNRGBA(x, y, blue)
			}
		}
	}

	set := func(x, y int) {
		if x >= 0 && x < sz && y >= 0 && y < sz {
			img.SetNRGBA(x, y, white)
		}
	}

	// Left port bar: x=[5,7], y=[9,22]
	for x := 5; x <= 7; x++ {
		for y := 9; y <= 22; y++ {
			set(x, y)
		}
	}

	// Right port bar: x=[24,26], y=[9,22]
	for x := 24; x <= 26; x++ {
		for y := 9; y <= 22; y++ {
			set(x, y)
		}
	}

	// Arrow shaft: x=[8,14], y=[14,17]
	for x := 8; x <= 14; x++ {
		for y := 14; y <= 17; y++ {
			set(x, y)
		}
	}

	// Arrow head: triangle, tip at (23, 15.5), base at x=14 with halfH=4.5
	for xi := 14; xi <= 23; xi++ {
		frac := float64(xi-14) / float64(23-14)
		halfH := (1.0 - frac) * 4.5
		y1 := int(math.Round(15.5 - halfH))
		y2 := int(math.Round(15.5 + halfH))
		for y := y1; y <= y2; y++ {
			set(xi, y)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
