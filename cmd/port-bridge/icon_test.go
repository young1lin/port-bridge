package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestMakeIconPNG(t *testing.T) {
	data := makeIconPNG()
	if len(data) == 0 {
		t.Fatal("makeIconPNG returned empty data")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("unexpected icon size: %v", img.Bounds())
	}

	center := img.At(16, 16)
	r, g, b, a := center.RGBA()
	if a == 0 {
		t.Fatal("center pixel should not be transparent")
	}
	if r == 0 && g == 0 && b == 0 {
		t.Fatal("center pixel should not be empty")
	}
}

func TestAppIconInitialized(t *testing.T) {
	if appIcon == nil {
		t.Fatal("appIcon should be initialized in init")
	}
	if appIcon.Name() != "icon.png" {
		t.Fatalf("unexpected icon name: %s", appIcon.Name())
	}
	if len(appIcon.Content()) == 0 {
		t.Fatal("appIcon content should not be empty")
	}
}
