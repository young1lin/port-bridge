package theme

import (
	"fmt"
	"image/color"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
)

func TestNewPortBridgeTheme_Defaults(t *testing.T) {
	th := NewPortBridgeTheme()
	if th == nil {
		t.Fatal("theme should not be nil")
	}
	if th.IsDark() {
		t.Fatal("new theme should default to light mode")
	}
	if th.Theme == nil {
		t.Fatal("embedded fyne theme should be set")
	}
	if th.Font(fyne.TextStyle{}) == nil {
		t.Fatal("font should fall back to a valid resource")
	}
}

func TestPortBridgeTheme_SetDark(t *testing.T) {
	th := NewPortBridgeTheme()
	th.SetDark(true)
	if !th.IsDark() {
		t.Fatal("SetDark(true) should enable dark mode")
	}
	th.SetDark(false)
	if th.IsDark() {
		t.Fatal("SetDark(false) should disable dark mode")
	}
}

func TestPortBridgeTheme_SizeOverrides(t *testing.T) {
	th := NewPortBridgeTheme()

	if got := th.Size(fynetheme.SizeNameInputRadius); got != 8 {
		t.Fatalf("unexpected input radius: %v", got)
	}
	if got := th.Size(fynetheme.SizeNameSelectionRadius); got != 4 {
		t.Fatalf("unexpected selection radius: %v", got)
	}
	if got := th.Size(fynetheme.SizeNamePadding); got != 4 {
		t.Fatalf("unexpected padding: %v", got)
	}
	if got := th.Size(fynetheme.SizeNameScrollBarRadius); got != 4 {
		t.Fatalf("unexpected scrollbar radius: %v", got)
	}
	if got := th.Size(fynetheme.SizeNameInnerPadding); got != 6 {
		t.Fatalf("unexpected inner padding: %v", got)
	}
	if got := th.Size(fynetheme.SizeNameText); got != th.Theme.Size(fynetheme.SizeNameText) {
		t.Fatalf("expected fallback size to default theme, got %v", got)
	}
}

func TestPortBridgeTheme_Colors(t *testing.T) {
	th := NewPortBridgeTheme()

	if got := color.NRGBAModel.Convert(th.Color(fynetheme.ColorNameBackground, fynetheme.VariantLight)).(color.NRGBA); got != (color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf9, A: 0xff}) {
		t.Fatalf("unexpected light background color: %#v", got)
	}

	th.SetDark(true)
	if got := color.NRGBAModel.Convert(th.Color(fynetheme.ColorNameBackground, fynetheme.VariantDark)).(color.NRGBA); got != (color.NRGBA{R: 0x1a, G: 0x1b, B: 0x26, A: 0xff}) {
		t.Fatalf("unexpected dark background color: %#v", got)
	}
}

func TestPortBridgeTheme_LightAndDarkColorTables(t *testing.T) {
	th := NewPortBridgeTheme()

	lightCases := map[fyne.ThemeColorName]color.NRGBA{
		fynetheme.ColorNameHeaderBackground:  {R: 0xee, G: 0xef, B: 0xf4, A: 0xff},
		fynetheme.ColorNameMenuBackground:    {R: 0xee, G: 0xef, B: 0xf4, A: 0xff},
		fynetheme.ColorNameOverlayBackground: {R: 0xf0, G: 0xf0, B: 0xf2, A: 0xe6},
		fynetheme.ColorNameButton:            {R: 0xe0, G: 0xe2, B: 0xe8, A: 0xff},
		fynetheme.ColorNameDisabledButton:    {R: 0xe0, G: 0xe2, B: 0xe8, A: 0xff},
		fynetheme.ColorNameInputBackground:   {R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		fynetheme.ColorNameInputBorder:       {R: 0xd0, G: 0xd2, B: 0xdb, A: 0xff},
		fynetheme.ColorNameForeground:        {R: 0x1a, G: 0x1c, B: 0x2b, A: 0xff},
		fynetheme.ColorNamePlaceHolder:       {R: 0x8e, G: 0x91, B: 0xa1, A: 0xff},
		fynetheme.ColorNameHover:             {R: 0x00, G: 0x00, B: 0x00, A: 0x0a},
		fynetheme.ColorNamePressed:           {R: 0x00, G: 0x00, B: 0x00, A: 0x14},
		fynetheme.ColorNameSeparator:         {R: 0xe0, G: 0xe1, B: 0xe6, A: 0xff},
		fynetheme.ColorNameShadow:            {R: 0x00, G: 0x00, B: 0x00, A: 0x18},
		fynetheme.ColorNameSuccess:           {R: 0x34, G: 0x95, B: 0x3a, A: 0xff},
		fynetheme.ColorNameWarning:           {R: 0xe6, G: 0x8a, B: 0x00, A: 0xff},
		fynetheme.ColorNameError:             {R: 0xd3, G: 0x30, B: 0x2f, A: 0xff},
	}

	for name, want := range lightCases {
		if got := color.NRGBAModel.Convert(th.lightColor(name)).(color.NRGBA); got != want {
			t.Fatalf("unexpected light color for %q: %#v", name, got)
		}
	}

	darkCases := map[fyne.ThemeColorName]color.NRGBA{
		fynetheme.ColorNameHeaderBackground:  {R: 0x1e, G: 0x1f, B: 0x2b, A: 0xff},
		fynetheme.ColorNameMenuBackground:    {R: 0x1e, G: 0x1f, B: 0x2b, A: 0xff},
		fynetheme.ColorNameOverlayBackground: {R: 0x12, G: 0x13, B: 0x1a, A: 0xe6},
		fynetheme.ColorNameButton:            {R: 0x2d, G: 0x2f, B: 0x3e, A: 0xff},
		fynetheme.ColorNameDisabledButton:    {R: 0x25, G: 0x26, B: 0x33, A: 0xff},
		fynetheme.ColorNameInputBackground:   {R: 0x22, G: 0x24, B: 0x33, A: 0xff},
		fynetheme.ColorNameInputBorder:       {R: 0x3a, G: 0x3d, B: 0x52, A: 0xff},
		fynetheme.ColorNameForeground:        {R: 0xe8, G: 0xea, B: 0xed, A: 0xff},
		fynetheme.ColorNamePlaceHolder:       {R: 0x6c, G: 0x70, B: 0x86, A: 0xff},
		fynetheme.ColorNameHover:             {R: 0xff, G: 0xff, B: 0xff, A: 0x0d},
		fynetheme.ColorNamePressed:           {R: 0xff, G: 0xff, B: 0xff, A: 0x1a},
		fynetheme.ColorNameSeparator:         {R: 0x32, G: 0x34, B: 0x44, A: 0xff},
		fynetheme.ColorNameShadow:            {R: 0x00, G: 0x00, B: 0x00, A: 0x33},
		fynetheme.ColorNameSuccess:           {R: 0x4c, G: 0xaf, B: 0x50, A: 0xff},
		fynetheme.ColorNameWarning:           {R: 0xff, G: 0xb7, B: 0x40, A: 0xff},
		fynetheme.ColorNameError:             {R: 0xef, G: 0x53, B: 0x50, A: 0xff},
	}

	for name, want := range darkCases {
		if got := color.NRGBAModel.Convert(th.darkColor(name)).(color.NRGBA); got != want {
			t.Fatalf("unexpected dark color for %q: %#v", name, got)
		}
	}
}

func TestPortBridgeTheme_FontAndIconFallbacks(t *testing.T) {
	th := NewPortBridgeTheme()
	th.font = nil

	if th.Font(fyne.TextStyle{}) == nil {
		t.Fatal("expected font fallback from embedded theme")
	}
	if th.Icon(fynetheme.IconNameCancel) == nil {
		t.Fatal("expected icon fallback from embedded theme")
	}
}

func TestLoadChineseFont_FindsResource(t *testing.T) {
	origGOOS := runtimeGOOS
	origStat := fileStat
	origLoad := loadResourceFromPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		fileStat = origStat
		loadResourceFromPath = origLoad
	})

	runtimeGOOS = "windows"
	fileStat = func(name string) (os.FileInfo, error) {
		if name == "C:\\Windows\\Fonts\\simhei.ttf" {
			return fakeFileInfo{name: "simhei.ttf"}, nil
		}
		return nil, os.ErrNotExist
	}
	loadResourceFromPath = func(path string) (fyne.Resource, error) {
		return fyne.NewStaticResource("font.ttf", []byte(path)), nil
	}

	res := loadChineseFont()
	if res == nil {
		t.Fatal("expected font resource")
	}
}

func TestLoadChineseFont_FallbacksAndFailures(t *testing.T) {
	origGOOS := runtimeGOOS
	origStat := fileStat
	origLoad := loadResourceFromPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		fileStat = origStat
		loadResourceFromPath = origLoad
	})

	tests := []string{"darwin", "linux", "unknown"}
	for _, goos := range tests {
		t.Run(goos, func(t *testing.T) {
			runtimeGOOS = goos
			fileStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
			loadResourceFromPath = func(path string) (fyne.Resource, error) {
				return nil, fmt.Errorf("load failed")
			}

			if res := loadChineseFont(); res != nil {
				t.Fatalf("expected nil font for %s", goos)
			}
		})
	}
}

type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }
