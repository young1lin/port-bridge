//go:build ignore

// gen_ico generates cmd/port-bridge/icon.ico from the SVG design at
// four standard sizes (16, 32, 48, 256).  Run with:
//
//	go run ./cmd/port-bridge/gen_ico/main.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	// Windows ICO: 16, 32, 48, 256 px frames
	icoSizes := []int{16, 32, 48, 256}
	var pngFrames [][]byte
	for _, sz := range icoSizes {
		img := drawIcon(sz)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		pngFrames = append(pngFrames, buf.Bytes())
	}
	ico := buildICO(icoSizes, pngFrames)
	if err := os.WriteFile("cmd/port-bridge/icon.ico", ico, 0644); err != nil {
		panic(err)
	}

	// macOS iconset source: 1024 × 1024 PNG
	img1024 := drawIcon(1024)
	var buf1024 bytes.Buffer
	if err := png.Encode(&buf1024, img1024); err != nil {
		panic(err)
	}
	if err := os.WriteFile("cmd/port-bridge/assets/icon_1024.png", buf1024.Bytes(), 0644); err != nil {
		panic(err)
	}
}

// drawIcon renders the port-bridge icon at the given square size.
// Design (matches assets/icon.svg):
//   - Blue circle background
//   - Left port bar  (local)
//   - Right port bar (remote)
//   - Two faint tunnel rails
//   - Solid arrow shaft + head
//   - Small SSH lock on the shaft
func drawIcon(sz int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, sz, sz))

	blue := color.NRGBA{21, 101, 192, 255}  // #1565C0
	white := color.NRGBA{255, 255, 255, 255}
	rail := color.NRGBA{255, 255, 255, 140} // white @ 55% opacity
	lock := blue                             // lock drawn in background colour

	s := func(v float64) int { return int(math.Round(v * float64(sz) / 100.0)) }

	set := func(x, y int, c color.NRGBA) {
		if x >= 0 && x < sz && y >= 0 && y < sz {
			img.SetNRGBA(x, y, c)
		}
	}

	fillRect := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				set(x, y, c)
			}
		}
	}

	// ── Background circle ────────────────────────────────────────────────────
	cx, cy := float64(sz)/2, float64(sz)/2
	r := float64(sz)/2 - 0.5
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if math.Sqrt(dx*dx+dy*dy) < r {
				img.SetNRGBA(x, y, blue)
			}
		}
	}

	// ── Left port bar  x=[10,20] y=[18,81] ──────────────────────────────────
	fillRect(s(10), s(18), s(20), s(81), white)

	// ── Right port bar  x=[79,89] y=[18,81] ─────────────────────────────────
	fillRect(s(79), s(18), s(89), s(81), white)

	// ── Tunnel rails (semi-transparent) ─────────────────────────────────────
	fillRect(s(21), s(38), s(78), s(44), rail)
	fillRect(s(21), s(55), s(78), s(61), rail)

	// ── Arrow shaft  x=[21,58] y=[43,56] ────────────────────────────────────
	fillRect(s(21), s(43), s(58), s(56), white)

	// ── Arrow head: triangle tip=(82,50), base x=59, half-height=22 ─────────
	for xi := s(59); xi <= s(82); xi++ {
		frac := float64(xi-s(59)) / float64(s(82)-s(59)+1)
		halfH := (1.0 - frac) * float64(s(22))
		y1 := int(math.Round(float64(s(50)) - halfH))
		y2 := int(math.Round(float64(s(50)) + halfH))
		for y := y1; y <= y2; y++ {
			set(xi, y, white)
		}
	}

	// ── SSH lock (only visible at ≥ 32 px) ──────────────────────────────────
	if sz >= 32 {
		// Lock body
		fillRect(s(43), s(44), s(52), s(52), lock)
		// Lock shackle: arc from (45,44) over (48,38) to (51,44)
		for t := 0.0; t <= math.Pi; t += 0.05 {
			lx := int(math.Round(float64(s(48)) + float64(s(3))*math.Cos(t)))
			ly := int(math.Round(float64(s(41)) - float64(s(4))*math.Sin(t)))
			set(lx, ly, lock)
			set(lx-1, ly, lock)
		}
		// Keyhole dot
		set(s(48), s(49), white)
		if sz >= 48 {
			set(s(48)+1, s(49), white)
			set(s(48), s(49)+1, white)
		}
	}

	return img
}

// buildICO packs PNG frames into a PNG-compressed ICO file.
// PNG-in-ICO is supported on Windows Vista and later.
func buildICO(sizes []int, pngFrames [][]byte) []byte {
	n := len(sizes)

	// Fixed header + directory
	headerBytes := 6 + n*16
	offsets := make([]int, n)
	off := headerBytes
	for i, data := range pngFrames {
		offsets[i] = off
		off += len(data)
	}

	var buf bytes.Buffer

	// ICO header
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // image type: icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(n))

	// Directory entries
	for i, sz := range sizes {
		w, h := uint8(sz), uint8(sz)
		if sz >= 256 {
			w, h = 0, 0 // 0 encodes 256 in the ICO format
		}
		_ = binary.Write(&buf, binary.LittleEndian, w)              // width
		_ = binary.Write(&buf, binary.LittleEndian, h)              // height
		_ = binary.Write(&buf, binary.LittleEndian, uint8(0))       // colour count (0 = no palette)
		_ = binary.Write(&buf, binary.LittleEndian, uint8(0))       // reserved
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))      // colour planes
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))     // bits per pixel
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngFrames[i])))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(offsets[i]))
	}

	// Image data
	for _, data := range pngFrames {
		buf.Write(data)
	}

	return buf.Bytes()
}
