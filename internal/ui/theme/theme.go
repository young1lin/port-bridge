package theme

import (
	"image/color"
	"os"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	runtimeGOOS          = runtime.GOOS
	fileStat             = os.Stat
	loadResourceFromPath = fyne.LoadResourceFromPath
)

// PortBridgeTheme is a custom theme with larger border radius and refined colors.
// It supports both light and dark variants controlled by the dark field.
type PortBridgeTheme struct {
	fyne.Theme
	font fyne.Resource
	dark bool
}

// NewPortBridgeTheme creates the application theme.
// By default it uses light mode; call SetDark(true) to switch.
func NewPortBridgeTheme() *PortBridgeTheme {
	t := &PortBridgeTheme{
		Theme: theme.DefaultTheme(),
		dark:  false,
	}
	t.font = loadChineseFont()
	return t
}

// SetDark switches between light and dark mode.
func (t *PortBridgeTheme) SetDark(dark bool) {
	t.dark = dark
}

// IsDark returns the current theme mode.
func (t *PortBridgeTheme) IsDark() bool {
	return t.dark
}

// Font returns the font resource for the specified style.
func (t *PortBridgeTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.font != nil {
		return t.font
	}
	return t.Theme.Font(style)
}

// Size overrides default sizes for larger border radius and padding.
func (t *PortBridgeTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameSelectionRadius:
		return 4
	case theme.SizeNameScrollBarRadius:
		return 4
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNamePadding:
		return 4
	default:
		return t.Theme.Size(name)
	}
}

// Color returns colors for the current variant (light or dark).
func (t *PortBridgeTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if t.dark {
		return t.darkColor(name)
	}
	return t.lightColor(name)
}

// Icon inherits from default theme.
func (t *PortBridgeTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.Theme.Icon(name)
}

func (t *PortBridgeTheme) lightColor(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf9, A: 0xff}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0xee, G: 0xef, B: 0xf4, A: 0xff}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0xee, G: 0xef, B: 0xf4, A: 0xff}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0xf0, G: 0xf0, B: 0xf2, A: 0xe6}

	case theme.ColorNameButton:
		return color.NRGBA{R: 0xe0, G: 0xe2, B: 0xe8, A: 0xff}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0xe0, G: 0xe2, B: 0xe8, A: 0xff}

	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 0xd0, G: 0xd2, B: 0xdb, A: 0xff}

	case theme.ColorNameForeground:
		return color.NRGBA{R: 0x1a, G: 0x1c, B: 0x2b, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x8e, G: 0x91, B: 0xa1, A: 0xff}

	case theme.ColorNameHover:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x0a}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x14}

	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0xe0, G: 0xe1, B: 0xe6, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x18}

	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x34, G: 0x95, B: 0x3a, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xe6, G: 0x8a, B: 0x00, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xd3, G: 0x30, B: 0x2f, A: 0xff}

	default:
		return t.Theme.Color(name, theme.VariantLight)
	}
}

func (t *PortBridgeTheme) darkColor(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x1a, G: 0x1b, B: 0x26, A: 0xff}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0x1e, G: 0x1f, B: 0x2b, A: 0xff}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0x1e, G: 0x1f, B: 0x2b, A: 0xff}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0x12, G: 0x13, B: 0x1a, A: 0xe6}

	case theme.ColorNameButton:
		return color.NRGBA{R: 0x2d, G: 0x2f, B: 0x3e, A: 0xff}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x25, G: 0x26, B: 0x33, A: 0xff}

	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x22, G: 0x24, B: 0x33, A: 0xff}
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 0x3a, G: 0x3d, B: 0x52, A: 0xff}

	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xe8, G: 0xea, B: 0xed, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x6c, G: 0x70, B: 0x86, A: 0xff}

	case theme.ColorNameHover:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0d}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x1a}

	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0x32, G: 0x34, B: 0x44, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x33}

	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xff, G: 0xb7, B: 0x40, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xef, G: 0x53, B: 0x50, A: 0xff}

	default:
		return t.Theme.Color(name, theme.VariantDark)
	}
}

// --- Chinese font loading ---

func loadChineseFont() fyne.Resource {
	var fontPaths []string

	switch runtimeGOOS {
	case "windows":
		fontPaths = []string{
			"C:\\Windows\\Fonts\\simhei.ttf",
			"C:\\Windows\\Fonts\\simfang.ttf",
			"C:\\Windows\\Fonts\\simsunb.ttf",
		}
	case "darwin":
		fontPaths = []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
		}
	case "linux":
		fontPaths = []string{
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		}
	}

	for _, path := range fontPaths {
		if _, err := fileStat(path); err == nil {
			res, err := loadResourceFromPath(path)
			if err == nil {
				return res
			}
		}
	}

	return nil
}
