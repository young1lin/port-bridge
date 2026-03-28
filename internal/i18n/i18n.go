package i18n

import (
	"embed"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var (
	mu          sync.Mutex
	changes     map[int]func()
	nextCBID    int
	bundle      *i18n.Bundle
	localizer   *i18n.Localizer
	currentLang string
)

func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	currentLang = "en"
	localizer = i18n.NewLocalizer(bundle, currentLang)
	changes = make(map[int]func())
}

// OnLanguageChange registers a callback that is invoked when the language changes.
// It returns an unregister function that removes the callback when called.
func OnLanguageChange(cb func()) (unregister func()) {
	mu.Lock()
	defer mu.Unlock()
	id := nextCBID
	nextCBID++
	changes[id] = cb
	return func() {
		mu.Lock()
		defer mu.Unlock()
		delete(changes, id)
	}
}

// NotifyLanguageChange fires all registered language change callbacks.
func NotifyLanguageChange() {
	mu.Lock()
	callbacks := make([]func(), 0, len(changes))
	for _, cb := range changes {
		callbacks = append(callbacks, cb)
	}
	mu.Unlock()

	for _, cb := range callbacks {
		cb()
	}
}

// AddTranslationsFS loads all translation JSON files from an embedded filesystem.
func AddTranslationsFS(fs embed.FS, dir string) error {
	files, err := fs.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		name := f.Name()
		data, err := fs.ReadFile(dir + "/" + name)
		if err != nil {
			log.Printf("[WARN] Failed to read translation file %s: %v", name, err)
			continue
		}

		if _, err := bundle.ParseMessageFileBytes(data, name); err != nil {
			log.Printf("[WARN] Failed to parse translation file %s: %v", name, err)
		}
	}

	return nil
}

// SetLanguage switches the active language and notifies all registered callbacks.
func SetLanguage(lang string) {
	lang = normalizeLang(lang)
	mu.Lock()
	currentLang = lang
	localizer = i18n.NewLocalizer(bundle, lang)
	mu.Unlock()
}

// GetLanguage returns the current language code.
func GetLanguage() string {
	mu.Lock()
	defer mu.Unlock()
	return currentLang
}

// L translates a key using the current locale.
// For formatted strings, use fmt.Sprintf(i18n.L("key"), args...).
func L(key string) string {
	mu.Lock()
	loc := localizer
	mu.Unlock()

	ret, err := loc.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    key,
			Other: key,
		},
	})

	if err != nil {
		return key
	}
	return ret
}

// StatusText translates a status string using the current locale.
// It is a convenience wrapper for translating model status strings.
func StatusText(statusString string) string {
	return L(statusString)
}

func normalizeLang(lang string) string {
	// Handle BCP 47 tags like "zh-CN" -> "zh"
	if idx := strings.Index(lang, "-"); idx > 0 {
		lang = lang[:idx]
	}
	return strings.ToLower(lang)
}
