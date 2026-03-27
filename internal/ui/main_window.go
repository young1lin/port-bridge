package ui

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	fynetheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/ui/dialogs"
	"github.com/young1lin/port-bridge/internal/ui/theme"
	"github.com/young1lin/port-bridge/internal/ui/views"
	"github.com/young1lin/port-bridge/internal/ui/windowguard"
	"github.com/young1lin/port-bridge/internal/updater"
	"github.com/young1lin/port-bridge/internal/version"
)

type updaterService interface {
	CheckForUpdate() (*updater.ReleaseInfo, error)
	CheckForUpdateWithCache(force bool) (*updater.ReleaseInfo, error)
	DownloadAndApply(release *updater.ReleaseInfo, progress updater.ProgressCallback) error
}

var (
	newUpdateChecker          = func() updaterService { return updater.NewUpdater() }
	showUpdateAvailableDialog = func(win fyne.Window, release *updater.ReleaseInfo, u updaterService) {
		dialogs.ShowUpdateAvailableDialog(win, release, u)
	}
	showError       = dialog.ShowError
	showInformation = dialog.ShowInformation
	newChildWindow  = func(app fyne.App, title string) childWindowController {
		return app.NewWindow(title)
	}
	setWindowMainMenu = func(window fyne.Window, menu *fyne.MainMenu) {
		window.SetMainMenu(menu)
	}
	useNativeMainMenu = func() bool {
		return runtime.GOOS == "darwin"
	}
	currentThemeVariant = func(app fyne.App) fyne.ThemeVariant {
		return app.Settings().ThemeVariant()
	}
	showAndRunWindow = func(window fyne.Window) {
		window.ShowAndRun()
	}
	uiTraceOnce sync.Once
	uiTraceLog  *log.Logger
)

const (
	uiTraceEnv     = "PORTBRIDGE_UI_TRACE"
	uiTraceFileEnv = "PORTBRIDGE_UI_TRACE_FILE"
)

type childWindowController interface {
	SetContent(content fyne.CanvasObject)
	Resize(size fyne.Size)
	SetFixedSize(fixed bool)
	CenterOnScreen()
	SetOnClosed(func())
	Show()
}

type fixedWindowLayout struct {
	size fyne.Size
}

func (l fixedWindowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		obj.Move(fyne.NewPos(0, 0))
		obj.Resize(size)
	}
}

func (l fixedWindowLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return l.size
}

func showFixedChildWindow(app fyne.App, parent fyne.Window, title string, content fyne.CanvasObject, size fyne.Size) {
	body := container.New(fixedWindowLayout{size: size}, container.NewPadded(content))
	traceDialogState(title, "before-create", parent, content, body)
	win := newChildWindow(app, title)
	unlockParent := windowguard.ProtectParentWindow(parent)
	win.SetOnClosed(unlockParent)
	win.SetContent(body)
	win.Resize(size)
	traceDialogState(title, "after-resize", parent, content, body)
	win.SetFixedSize(true)
	win.CenterOnScreen()
	win.Show()
	traceDialogState(title, "after-show", parent, content, body)
	scheduleDialogTrace(title, parent, content, body)
}

func uiTraceEnabled() bool {
	return os.Getenv(uiTraceEnv) != ""
}

func getUITraceLogger() *log.Logger {
	if !uiTraceEnabled() {
		return nil
	}
	uiTraceOnce.Do(func() {
		tracePath := os.Getenv(uiTraceFileEnv)
		if tracePath == "" {
			tracePath = filepath.Join(os.TempDir(), "port-bridge-ui-trace.log")
		}
		if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
			log.Printf("[WARN] ui trace mkdir failed for %s: %v", tracePath, err)
			return
		}
		file, err := os.OpenFile(tracePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Printf("[WARN] ui trace open failed for %s: %v", tracePath, err)
			return
		}
		uiTraceLog = log.New(file, "[UI-TRACE] ", log.LstdFlags|log.Lmicroseconds)
		uiTraceLog.Printf("trace enabled path=%s pid=%d", tracePath, os.Getpid())
	})
	return uiTraceLog
}

func traceDialogState(title, stage string, parent fyne.Window, content, body fyne.CanvasObject) {
	logger := getUITraceLogger()
	if logger == nil {
		return
	}
	parentContent := parent.Content()
	logger.Printf(
		"title=%q stage=%s canvas_size=%v window_content_size=%v window_content_min=%v dialog_content_size=%v dialog_content_min=%v dialog_body_size=%v dialog_body_min=%v",
		title,
		stage,
		parent.Canvas().Size(),
		sizeOf(parentContent),
		minSizeOf(parentContent),
		sizeOf(content),
		minSizeOf(content),
		sizeOf(body),
		minSizeOf(body),
	)
	traceObjectTree(logger, title+"/"+stage, parentContent, 0)
	traceObjectTree(logger, title+"/"+stage, content, 0)
}

func scheduleDialogTrace(title string, parent fyne.Window, content, body fyne.CanvasObject) {
	logger := getUITraceLogger()
	if logger == nil {
		return
	}
	delays := []time.Duration{
		50 * time.Millisecond,
		150 * time.Millisecond,
		300 * time.Millisecond,
		600 * time.Millisecond,
		1200 * time.Millisecond,
	}
	logger.Printf("title=%q stage=trace-scheduled delays=%v", title, delays)
	go func() {
		start := time.Now()
		for _, delay := range delays {
			time.Sleep(delay)
			elapsed := time.Since(start)
			fyne.DoAndWait(func() {
				traceDialogState(title, fmt.Sprintf("tick-%dms", elapsed.Milliseconds()), parent, content, body)
			})
		}
	}()
}

func traceObjectTree(logger *log.Logger, prefix string, obj fyne.CanvasObject, depth int) {
	if logger == nil || obj == nil || depth > 3 {
		return
	}
	logger.Printf(
		"%s depth=%d type=%T size=%v min=%v visible=%t",
		prefix,
		depth,
		obj,
		sizeOf(obj),
		minSizeOf(obj),
		obj.Visible(),
	)
	switch typed := obj.(type) {
	case *fyne.Container:
		for idx, child := range typed.Objects {
			traceObjectTree(logger, fmt.Sprintf("%s[%d]", prefix, idx), child, depth+1)
		}
	case *container.Scroll:
		traceObjectTree(logger, prefix+"/scroll", typed.Content, depth+1)
	}
}

func setSelectValueWithoutNotify(sel *widget.Select, value string) {
	if sel == nil {
		return
	}
	onChanged := sel.OnChanged
	sel.OnChanged = nil
	sel.SetSelected(value)
	sel.OnChanged = onChanged
}

func sizeOf(obj fyne.CanvasObject) fyne.Size {
	if obj == nil {
		return fyne.Size{}
	}
	return obj.Size()
}

func minSizeOf(obj fyne.CanvasObject) fyne.Size {
	if obj == nil {
		return fyne.Size{}
	}
	return obj.MinSize()
}

// languageLabels maps locale codes to display labels.
var languageLabels = map[string]string{
	"en": "English",
	"zh": "中文",
}

// labelToLocale maps display labels back to locale codes.
var labelToLocale map[string]string

// langOptions is the ordered list of language display labels.
var langOptions []string

func init() {
	labelToLocale = make(map[string]string, len(languageLabels))
	for code, label := range languageLabels {
		labelToLocale[label] = code
	}
	langOptions = make([]string, 0, len(languageLabels))
	for _, label := range languageLabels {
		langOptions = append(langOptions, label)
	}
}

// themeLabels maps preference values to display labels (order matters).
var themeLabels = []struct {
	value string
	label string
}{
	{"system", "System"},
	{"light", "Light"},
	{"dark", "Dark"},
}

// closeActionLabels maps preference values to display labels (order matters).
var closeActionLabels = []struct {
	value string
	label string
}{
	{"ask", "Ask every time"},
	{"minimize", "Minimize to system tray"},
	{"exit", "Exit program"},
}

// MainWindow represents the main application window
type MainWindow struct {
	app          fyne.App
	window       fyne.Window
	connView     *views.ConnectionView
	tunnelView   *views.TunnelView
	tabs         *container.AppTabs
	connTab      *container.TabItem
	tunnelTab    *container.TabItem
	topBar       fyne.CanvasObject
	appTheme     *theme.PortBridgeTheme
	versionLabel *widget.Label
}

// NewMainWindow creates a new main window
func NewMainWindow(app fyne.App, appTheme *theme.PortBridgeTheme) *MainWindow {
	mw := &MainWindow{
		app:      app,
		appTheme: appTheme,
	}

	mw.window = app.NewWindow(i18n.L("SSH Port Forward Tool"))
	mw.window.Resize(fyne.NewSize(800, 500))
	mw.window.SetFixedSize(true)
	mw.window.CenterOnScreen()

	mw.setupUI()
	mw.setupMainMenu()

	// Follow system theme changes when mode is "system"
	settingsCh := make(chan fyne.Settings, 1)
	mw.app.Settings().AddChangeListener(settingsCh)
	go func() {
		for range settingsCh {
			if mw.app.Preferences().StringWithFallback("theme_mode", "system") == "system" {
				mw.appTheme.SetDark(currentThemeVariant(mw.app) == fynetheme.VariantDark)
				fyne.Do(func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[WARN] theme apply skipped during shutdown: %v", r)
						}
					}()
					mw.app.Settings().SetTheme(mw.appTheme)
				})
			}
		}
	}()

	i18n.OnLanguageChange(mw.onLanguageChange)

	return mw
}

// setupUI sets up the main UI
func (mw *MainWindow) setupUI() {
	mw.connTab = container.NewTabItem(i18n.L("SSH Connections"), widget.NewLabel("Loading..."))
	mw.tunnelTab = container.NewTabItem(i18n.L("Port Forwarding"), widget.NewLabel("Loading..."))
	mw.tabs = container.NewAppTabs(mw.connTab, mw.tunnelTab)

	mw.versionLabel = widget.NewLabel(version.ShortVersion())
	mw.versionLabel.Importance = widget.MediumImportance

	bottomBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), mw.versionLabel),
	)
	mw.window.SetContent(container.NewBorder(mw.topBar, bottomBar, nil, nil, mw.tabs))
}

// SetViews sets the connection and tunnel views
func (mw *MainWindow) SetViews(connView *views.ConnectionView, tunnelView *views.TunnelView) {
	mw.connView = connView
	mw.tunnelView = tunnelView

	mw.connTab = container.NewTabItem(i18n.L("SSH Connections"), connView.Container())
	mw.tunnelTab = container.NewTabItem(i18n.L("Port Forwarding"), tunnelView.Container())
	mw.tabs = container.NewAppTabs(mw.connTab, mw.tunnelTab)

	bottomBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), mw.versionLabel),
	)
	mw.window.SetContent(container.NewBorder(mw.topBar, bottomBar, nil, nil, mw.tabs))
}

// setupMainMenu creates the application menu bar.
func (mw *MainWindow) setupMainMenu() {
	if !useNativeMainMenu() {
		mw.topBar = mw.buildActionBar()
		setWindowMainMenu(mw.window, nil)
		mw.refreshContent()
		return
	}

	mw.topBar = nil
	quitItem := fyne.NewMenuItem(i18n.L("Quit"), func() {
		mw.app.Quit()
	})
	quitItem.IsQuit = true

	menu := fyne.NewMainMenu(
		fyne.NewMenu(i18n.L("Settings"),
			fyne.NewMenuItem(i18n.L("Preferences..."), mw.showSettingsDialog),
			fyne.NewMenuItemSeparator(),
			quitItem,
		),
		fyne.NewMenu(i18n.L("Help"),
			fyne.NewMenuItem(i18n.L("Check for Updates..."), mw.onCheckForUpdates),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(i18n.L("About PortBridge"), mw.showAboutDialog),
		),
	)
	setWindowMainMenu(mw.window, menu)
	mw.refreshContent()
}

func (mw *MainWindow) buildActionBar() fyne.CanvasObject {
	settingsBtn := widget.NewButton(i18n.L("Settings"), func() {})
	helpBtn := widget.NewButton(i18n.L("Help"), func() {})
	settingsBtn.Importance = widget.LowImportance
	helpBtn.Importance = widget.LowImportance

	settingsMenu := fyne.NewMenu("",
		fyne.NewMenuItem(i18n.L("Preferences..."), mw.showSettingsDialog),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.L("Quit"), func() {
			mw.app.Quit()
		}),
	)
	helpMenu := fyne.NewMenu("",
		fyne.NewMenuItem(i18n.L("Check for Updates..."), mw.onCheckForUpdates),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.L("About PortBridge"), mw.showAboutDialog),
	)

	settingsBtn.OnTapped = func() {
		widget.ShowPopUpMenuAtPosition(settingsMenu, mw.window.Canvas(), fyne.NewPos(12, settingsBtn.Size().Height+12))
	}
	helpBtn.OnTapped = func() {
		widget.ShowPopUpMenuAtPosition(helpMenu, mw.window.Canvas(), fyne.NewPos(110, helpBtn.Size().Height+12))
	}

	return container.NewVBox(
		container.NewPadded(container.NewHBox(settingsBtn, helpBtn, layout.NewSpacer())),
		widget.NewSeparator(),
	)
}

func (mw *MainWindow) refreshContent() {
	if mw.tabs == nil || mw.versionLabel == nil {
		return
	}
	bottomBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), mw.versionLabel),
	)
	mw.window.SetContent(container.NewBorder(mw.topBar, bottomBar, nil, nil, mw.tabs))
}

// onLanguageChange refreshes the main window when the language changes.
func (mw *MainWindow) onLanguageChange() {
	mw.window.SetTitle(i18n.L("SSH Port Forward Tool"))

	if mw.connTab != nil {
		mw.connTab.Text = i18n.L("SSH Connections")
	}
	if mw.tunnelTab != nil {
		mw.tunnelTab.Text = i18n.L("Port Forwarding")
	}
	if mw.tabs != nil {
		mw.tabs.Refresh()
	}
	if mw.versionLabel != nil {
		mw.versionLabel.SetText(version.ShortVersion())
	}

	mw.setupMainMenu()
}

// showSettingsDialog opens the Preferences dialog with language and theme selectors.
func (mw *MainWindow) showSettingsDialog() {
	// Language selector
	currentLang := mw.app.Preferences().StringWithFallback("language", "en")
	langSelect := widget.NewSelect(langOptions, func(selected string) {
		if code, ok := labelToLocale[selected]; ok {
			mw.app.Preferences().SetString("language", code)
			i18n.SetLanguage(code)
			i18n.NotifyLanguageChange()
		}
	})
	setSelectValueWithoutNotify(langSelect, languageLabels[currentLang])

	// Theme selector
	themeOptionLabels := make([]string, len(themeLabels))
	themeOptionToValue := make(map[string]string)
	for i, tl := range themeLabels {
		themeOptionLabels[i] = tl.label
		themeOptionToValue[tl.label] = tl.value
	}
	currentThemeVal := mw.app.Preferences().StringWithFallback("theme_mode", "system")
	themeSelect := widget.NewSelect(themeOptionLabels, func(selected string) {
		mode := themeOptionToValue[selected]
		mw.app.Preferences().SetString("theme_mode", mode)
		switch mode {
		case "dark":
			mw.appTheme.SetDark(true)
		case "light":
			mw.appTheme.SetDark(false)
		default: // "system"
			mw.appTheme.SetDark(currentThemeVariant(mw.app) == fynetheme.VariantDark)
		}
		mw.app.Settings().SetTheme(mw.appTheme)
	})
	currentThemeLabel := themeOptionLabels[0]
	for _, tl := range themeLabels {
		if tl.value == currentThemeVal {
			currentThemeLabel = tl.label
			break
		}
	}
	setSelectValueWithoutNotify(themeSelect, currentThemeLabel)

	// Auto-update checkbox
	autoCheck := widget.NewCheck(i18n.L("Check for updates on startup"), func(checked bool) {
		mw.app.Preferences().SetBool("auto_check_updates", checked)
	})
	autoCheck.Checked = mw.app.Preferences().BoolWithFallback("auto_check_updates", true)

	// Close window behavior selector
	closeOptions := make([]string, len(closeActionLabels))
	closeOptionToValue := make(map[string]string)
	for i, cl := range closeActionLabels {
		lbl := i18n.L(cl.label)
		closeOptions[i] = lbl
		closeOptionToValue[lbl] = cl.value
	}
	currentCloseValue := "ask"
	if mw.app.Preferences().Bool("close_action_saved") {
		currentCloseValue = mw.app.Preferences().StringWithFallback("close_action", "minimize")
	}
	closeSelect := widget.NewSelect(closeOptions, func(selected string) {
		action := closeOptionToValue[selected]
		switch action {
		case "ask":
			mw.app.Preferences().SetBool("close_action_saved", false)
		case "minimize":
			mw.app.Preferences().SetBool("close_action_saved", true)
			mw.app.Preferences().SetString("close_action", "minimize")
		case "exit":
			mw.app.Preferences().SetBool("close_action_saved", true)
			mw.app.Preferences().SetString("close_action", "exit")
		}
	})
	currentCloseLabel := closeOptions[0]
	for _, cl := range closeActionLabels {
		if cl.value == currentCloseValue {
			currentCloseLabel = i18n.L(cl.label)
			break
		}
	}
	setSelectValueWithoutNotify(closeSelect, currentCloseLabel)

	// Language row
	langRow := container.NewBorder(nil, nil,
		widget.NewLabel(i18n.L("Language:")), nil,
		langSelect,
	)

	// Theme row
	themeRow := container.NewBorder(nil, nil,
		widget.NewLabel(i18n.L("Theme:")), nil,
		themeSelect,
	)

	// Close window behavior row
	closeRow := container.NewBorder(nil, nil,
		widget.NewLabel(i18n.L("When closing window:")), nil,
		closeSelect,
	)

	content := container.NewVBox(langRow, themeRow, closeRow, widget.NewSeparator(), autoCheck)
	showFixedChildWindow(mw.app, mw.window, i18n.L("Preferences"), content, fyne.NewSize(520, 380))
}

// showAboutDialog displays the About PortBridge dialog.
func (mw *MainWindow) showAboutDialog() {
	icon := mw.app.Icon()
	title := widget.NewLabel("PortBridge")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	versionInfo := widget.NewLabel(fmt.Sprintf(
		"%s\n%s\n%s",
		version.ShortVersion(),
		"Commit: "+version.GitCommit,
		"Built: "+version.BuildDate,
	))
	versionInfo.Alignment = fyne.TextAlignCenter
	versionInfo.Importance = widget.MediumImportance

	link := widget.NewHyperlink(
		"github.com/"+version.RepoOwner+"/"+version.RepoName,
		parseURL("https://github.com/"+version.RepoOwner+"/"+version.RepoName),
	)
	link.Alignment = fyne.TextAlignCenter

	var content fyne.CanvasObject
	if icon != nil {
		iconWidget := widget.NewIcon(icon)
		iconContainer := container.NewGridWrap(fyne.NewSize(64, 64), iconWidget)
		content = container.NewVBox(
			container.NewCenter(iconContainer),
			title,
			versionInfo,
			link,
		)
	} else {
		content = container.NewVBox(
			title,
			versionInfo,
			link,
		)
	}

	showFixedChildWindow(mw.app, mw.window, i18n.L("About PortBridge"), content, fyne.NewSize(520, 300))
}

// onCheckForUpdates manually checks for updates and shows a result dialog.
func (mw *MainWindow) onCheckForUpdates() {
	// Use RunWithLoading for better UX
	var release *updater.ReleaseInfo
	RunWithLoading(mw.window, i18n.L("Check for Updates..."), i18n.L("Checking for updates..."), 30*time.Second, func(ctx context.Context) error {
		u := newUpdateChecker()
		var err error
		release, err = u.CheckForUpdateWithCache(true) // Force fresh check
		return err
	}, func(result Result, err error) {
		fyne.Do(func() {
			if err != nil {
				showError(fmt.Errorf(i18n.L("Update failed: %v"), err), mw.window)
				return
			}
			if release == nil {
				showInformation(i18n.L("Check for Updates..."), i18n.L("You're up to date!"), mw.window)
				return
			}
			u := newUpdateChecker()
			showUpdateAvailableDialog(mw.window, release, u)
		})
	})
}

// Show shows the main window
func (mw *MainWindow) Show() {
	showAndRunWindow(mw.window)
}

// GetWindow returns the underlying window
func (mw *MainWindow) GetWindow() fyne.Window {
	return mw.window
}

// SetOnClose sets the close callback
func (mw *MainWindow) SetOnClose(callback func()) {
	mw.window.SetCloseIntercept(callback)
}

// parseURL is a simple helper to parse a URL string.
func parseURL(urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		log.Printf("[WARN] Failed to parse URL %s: %v", urlStr, err)
		return nil
	}
	return u
}
