package i18n

import (
	"embed"
	"encoding/json"
	"sync"
	"testing"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed testdata/valid/*.json testdata/invalid/*.json testdata/mixed
var testTranslationsFS embed.FS

func resetBundleState(t *testing.T) {
	t.Helper()

	mu.Lock()
	origBundle := bundle
	origLocalizer := localizer
	origCurrentLang := currentLang
	origChanges := changes

	bundle = goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	currentLang = "en"
	localizer = goi18n.NewLocalizer(bundle, currentLang)
	changes = nil
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		bundle = origBundle
		localizer = origLocalizer
		currentLang = origCurrentLang
		changes = origChanges
		mu.Unlock()
	})
}

func TestNormalizeLang(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"en", "en"},
		{"zh", "zh"},
		{"zh-CN", "zh"},
		{"en-US", "en"},
		{"ZH-CN", "zh"},
		{"EN-US", "en"},
		{"", ""},
		{"pt-BR", "pt"},
	}
	for _, tt := range tests {
		got := normalizeLang(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLang(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetLanguage_Default(t *testing.T) {
	// init() sets currentLang to "en"
	got := GetLanguage()
	if got != "en" {
		t.Errorf("GetLanguage() = %q, want %q", got, "en")
	}
}

func TestSetLanguage(t *testing.T) {
	orig := currentLang
	origLocalizer := localizer
	defer func() {
		mu.Lock()
		currentLang = orig
		localizer = origLocalizer
		mu.Unlock()
	}()

	SetLanguage("zh")
	if got := GetLanguage(); got != "zh" {
		t.Errorf("GetLanguage() after SetLanguage(zh) = %q, want %q", got, "zh")
	}
}

func TestL_FallbackToKey(t *testing.T) {
	// Unknown key should return the key itself as fallback
	got := L("SomeUnknownTranslationKey")
	if got != "SomeUnknownTranslationKey" {
		t.Errorf("L(unknown) = %q, want %q", got, "SomeUnknownTranslationKey")
	}
}

func TestStatusText(t *testing.T) {
	got := StatusText("Connected")
	if got != "Connected" {
		t.Errorf("StatusText = %q, want %q", got, "Connected")
	}
}

func TestOnLanguageChange(t *testing.T) {
	// Save and restore original callbacks
	mu.Lock()
	orig := changes
	changes = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		changes = orig
		mu.Unlock()
	}()

	var called bool
	OnLanguageChange(func() { called = true })

	NotifyLanguageChange()

	if !called {
		t.Error("callback should have been called")
	}
}

func TestOnLanguageChange_Multiple(t *testing.T) {
	mu.Lock()
	orig := changes
	changes = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		changes = orig
		mu.Unlock()
	}()

	var count int
	var mu sync.Mutex
	OnLanguageChange(func() {
		mu.Lock()
		count++
		mu.Unlock()
	})
	OnLanguageChange(func() {
		mu.Lock()
		count++
		mu.Unlock()
	})

	NotifyLanguageChange()

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 callbacks, got %d", count)
	}
}

func TestSetLanguage_BCP47(t *testing.T) {
	orig := currentLang
	origLocalizer := localizer
	defer func() {
		mu.Lock()
		currentLang = orig
		localizer = origLocalizer
		mu.Unlock()
	}()

	SetLanguage("zh-CN")
	if got := GetLanguage(); got != "zh" {
		t.Errorf("GetLanguage() after SetLanguage(zh-CN) = %q, want %q", got, "zh")
	}
}

func TestSetLanguage_UpperCase(t *testing.T) {
	orig := currentLang
	origLocalizer := localizer
	defer func() {
		mu.Lock()
		currentLang = orig
		localizer = origLocalizer
		mu.Unlock()
	}()

	SetLanguage("EN-US")
	if got := GetLanguage(); got != "en" {
		t.Errorf("GetLanguage() after SetLanguage(EN-US) = %q, want %q", got, "en")
	}
}

func TestNotifyLanguageChange_WithSet(t *testing.T) {
	mu.Lock()
	orig := changes
	changes = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		changes = orig
		mu.Unlock()
	}()

	OnLanguageChange(func() { /* no-op */ })

	SetLanguage("zh")
	// SetLanguage does NOT call NotifyLanguageChange
	// This tests that SetLanguage and NotifyLanguageChange are independent
}

func TestAddTranslationsFS_Empty(t *testing.T) {
	// embed.FS cannot be constructed at runtime, test with empty FS
	err := AddTranslationsFS(embed.FS{}, "translations")
	// Function handles error internally (logs warning) and returns nil
	if err != nil {
		t.Logf("AddTranslationsFS with empty FS: %v", err)
	}
}

func TestAddTranslationsFS_NonexistentDir(t *testing.T) {
	err := AddTranslationsFS(embed.FS{}, "nonexistent-dir")
	// Function handles error internally
	_ = err
}

func TestAddTranslationsFS_LoadsMessages(t *testing.T) {
	resetBundleState(t)

	if err := AddTranslationsFS(testTranslationsFS, "testdata/valid"); err != nil {
		t.Fatalf("AddTranslationsFS: %v", err)
	}

	SetLanguage("zh")
	if got := L("Connected"); got != "已连接" {
		t.Fatalf("L(Connected) = %q, want %q", got, "已连接")
	}

	SetLanguage("en")
	if got := L("Connected"); got != "Connected" {
		t.Fatalf("L(Connected) = %q, want %q", got, "Connected")
	}
}

func TestAddTranslationsFS_InvalidFileDoesNotFail(t *testing.T) {
	resetBundleState(t)

	if err := AddTranslationsFS(testTranslationsFS, "testdata/invalid"); err != nil {
		t.Fatalf("AddTranslationsFS should ignore invalid files, got: %v", err)
	}
}

func TestAddTranslationsFS_ReadErrorDoesNotFail(t *testing.T) {
	resetBundleState(t)

	if err := AddTranslationsFS(testTranslationsFS, "testdata/mixed"); err != nil {
		t.Fatalf("AddTranslationsFS should ignore unreadable entries, got: %v", err)
	}
}

func TestL_WithBundle(t *testing.T) {
	// Test that L returns the key when no translation is found
	result := L("app.title")
	if result == "" {
		t.Error("L should never return empty string")
	}
	// It returns the key itself as fallback
	if result != "app.title" {
		t.Errorf("L(app.title) = %q, want %q", result, "app.title")
	}
}

func TestStatusText_Fallback(t *testing.T) {
	result := StatusText("Disconnected")
	if result != "Disconnected" {
		t.Errorf("StatusText(Disconnected) = %q, want %q", result, "Disconnected")
	}
}
