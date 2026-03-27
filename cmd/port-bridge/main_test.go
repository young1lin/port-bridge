package main

import (
	"embed"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/ui"
	uistyle "github.com/young1lin/port-bridge/internal/ui/theme"
	"github.com/young1lin/port-bridge/internal/updater"
)

type fakeMainUpdater struct {
	release *updater.ReleaseInfo
	err     error
}

func (f *fakeMainUpdater) CheckForUpdate() (*updater.ReleaseInfo, error) {
	return f.release, f.err
}

func (f *fakeMainUpdater) DownloadAndApply(*updater.ReleaseInfo, updater.ProgressCallback) error {
	return nil
}

func TestSetupUI_AppliesPreferencesAndBuildsViews(t *testing.T) {
	origNewFyneApp := newFyneApp
	origAddTranslationsFS := addTranslationsFS
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		addTranslationsFS = origAddTranslationsFS
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetString("language", "zh")
	testApp.Preferences().SetString("theme_mode", "dark")

	newFyneApp = func() fyne.App { return testApp }

	var translationsPath string
	addTranslationsFS = func(_ embed.FS, path string) error {
		translationsPath = path
		return nil
	}

	fyneApp, mainWindow, connView, tunnelView := setupUI()
	t.Cleanup(fyneApp.Quit)

	if fyneApp == nil || mainWindow == nil || connView == nil || tunnelView == nil {
		t.Fatal("setupUI should return app, main window, and both views")
	}
	if translationsPath != "translations" {
		t.Fatalf("expected translations path 'translations', got %q", translationsPath)
	}
	if i18n.GetLanguage() != "zh" {
		t.Fatalf("expected saved language to be applied, got %q", i18n.GetLanguage())
	}

	appTheme, ok := fyneApp.Settings().Theme().(*uistyle.PortBridgeTheme)
	if !ok {
		t.Fatalf("expected PortBridgeTheme, got %T", fyneApp.Settings().Theme())
	}
	if !appTheme.IsDark() {
		t.Fatal("expected dark theme mode from preferences")
	}
	if mainWindow.GetWindow() == nil {
		t.Fatal("main window should expose a fyne window")
	}
}

func TestSetupUI_TranslationLoadErrorDoesNotFail(t *testing.T) {
	origNewFyneApp := newFyneApp
	origAddTranslationsFS := addTranslationsFS
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		addTranslationsFS = origAddTranslationsFS
	})

	testApp := fynetest.NewApp()
	newFyneApp = func() fyne.App { return testApp }
	addTranslationsFS = func(_ embed.FS, path string) error {
		return errors.New("bad translations")
	}

	fyneApp, mainWindow, connView, tunnelView := setupUI()
	t.Cleanup(fyneApp.Quit)

	if fyneApp == nil || mainWindow == nil || connView == nil || tunnelView == nil {
		t.Fatal("setupUI should still succeed when translations fail to load")
	}
}

func TestRunApp_Success(t *testing.T) {
	origNewFyneApp := newFyneApp
	origNewCoreApp := newCoreApp
	origSetupTray := setupTrayFunc
	origHandleClose := handleCloseFunc
	origShowMain := showMainWindow
	origInitLogger := initLogger
	origNewUpdater := newUpdater
	origShowUpdate := showUpdateDialog
	origDelay := updateCheckDelay
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		newCoreApp = origNewCoreApp
		setupTrayFunc = origSetupTray
		handleCloseFunc = origHandleClose
		showMainWindow = origShowMain
		initLogger = origInitLogger
		newUpdater = origNewUpdater
		showUpdateDialog = origShowUpdate
		updateCheckDelay = origDelay
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetBool("auto_check_updates", false)
	newFyneApp = func() fyne.App { return testApp }
	dir := t.TempDir()
	newCoreApp = func() (*appcore.App, error) { return appcore.NewAppAt(dir) }

	trayCalled := 0
	showCalled := 0
	initLogger = func(string) error { return nil }
	setupTrayFunc = func(_ fyne.App, _ fyne.Window, _ func(), _ *TrayCallbacks) { trayCalled++ }
	handleCloseFunc = func(_ fyne.Window, _ fyne.Preferences, _ func(), _ func()) {}
	showMainWindow = func(mainWindow *ui.MainWindow) {
		showCalled++
	}

	if err := runApp(); err != nil {
		t.Fatalf("runApp returned error: %v", err)
	}

	if trayCalled != 1 {
		t.Fatalf("expected tray setup once, got %d", trayCalled)
	}
	if showCalled != 1 {
		t.Fatalf("expected main window show once, got %d", showCalled)
	}
}

func TestRunApp_InitLoggerWarning(t *testing.T) {
	origNewFyneApp := newFyneApp
	origNewCoreApp := newCoreApp
	origSetupTray := setupTrayFunc
	origHandleClose := handleCloseFunc
	origShowMain := showMainWindow
	origInitLogger := initLogger
	origNewUpdater := newUpdater
	origSetClose := setMainWindowClose
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		newCoreApp = origNewCoreApp
		setupTrayFunc = origSetupTray
		handleCloseFunc = origHandleClose
		showMainWindow = origShowMain
		initLogger = origInitLogger
		newUpdater = origNewUpdater
		setMainWindowClose = origSetClose
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetBool("auto_check_updates", false)
	newFyneApp = func() fyne.App { return testApp }
	dir := t.TempDir()
	newCoreApp = func() (*appcore.App, error) { return appcore.NewAppAt(dir) }
	setupTrayFunc = func(_ fyne.App, _ fyne.Window, _ func(), _ *TrayCallbacks) {}
	initLogger = func(string) error { return errors.New("log init failed") }
	handleCloseFunc = func(_ fyne.Window, _ fyne.Preferences, _ func(), _ func()) {}
	showMainWindow = func(*ui.MainWindow) {}

	if err := runApp(); err != nil {
		t.Fatalf("runApp returned error: %v", err)
	}
}

func TestRunApp_WindowCloseUsesHandleClose(t *testing.T) {
	origNewFyneApp := newFyneApp
	origNewCoreApp := newCoreApp
	origSetupTray := setupTrayFunc
	origHandleClose := handleCloseFunc
	origShowMain := showMainWindow
	origInitLogger := initLogger
	origSetClose := setMainWindowClose
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		newCoreApp = origNewCoreApp
		setupTrayFunc = origSetupTray
		handleCloseFunc = origHandleClose
		showMainWindow = origShowMain
		initLogger = origInitLogger
		setMainWindowClose = origSetClose
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetBool("auto_check_updates", false)
	newFyneApp = func() fyne.App { return testApp }
	dir := t.TempDir()
	newCoreApp = func() (*appcore.App, error) { return appcore.NewAppAt(dir) }
	setupTrayFunc = func(_ fyne.App, _ fyne.Window, _ func(), _ *TrayCallbacks) {}
	initLogger = func(string) error { return nil }

	closeHandlerSet := 0
	closeHandlerCalled := 0
	setMainWindowClose = func(_ *ui.MainWindow, callback func()) {
		closeHandlerSet++
		showMainWindow = func(*ui.MainWindow) {
			callback()
		}
	}
	handleCloseFunc = func(_ fyne.Window, prefs fyne.Preferences, onMinimize, _ func()) {
		closeHandlerCalled++
		if prefs == nil {
			t.Fatal("expected preferences to be passed to close handler")
		}
		onMinimize()
	}

	if err := runApp(); err != nil {
		t.Fatalf("runApp returned error: %v", err)
	}
	if closeHandlerSet != 1 {
		t.Fatalf("expected close handler to be registered once, got %d", closeHandlerSet)
	}
	if closeHandlerCalled != 1 {
		t.Fatalf("expected close handler callback to run once, got %d", closeHandlerCalled)
	}
}

func TestRunApp_TrayExitHook(t *testing.T) {
	origNewFyneApp := newFyneApp
	origNewCoreApp := newCoreApp
	origSetupTray := setupTrayFunc
	origHandleClose := handleCloseFunc
	origShowMain := showMainWindow
	origInitLogger := initLogger
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		newCoreApp = origNewCoreApp
		setupTrayFunc = origSetupTray
		handleCloseFunc = origHandleClose
		showMainWindow = origShowMain
		initLogger = origInitLogger
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetBool("auto_check_updates", false)
	newFyneApp = func() fyne.App { return testApp }
	dir := t.TempDir()
	newCoreApp = func() (*appcore.App, error) { return appcore.NewAppAt(dir) }
	initLogger = func(string) error { return nil }
	handleCloseFunc = func(_ fyne.Window, _ fyne.Preferences, _ func(), _ func()) {}
	showMainWindow = func(*ui.MainWindow) {}

	exitCalled := 0
	setupTrayFunc = func(_ fyne.App, _ fyne.Window, onExit func(), _ *TrayCallbacks) {
		exitCalled++
		onExit()
	}

	if err := runApp(); err != nil {
		t.Fatalf("runApp returned error: %v", err)
	}
	if exitCalled != 1 {
		t.Fatalf("expected tray exit hook once, got %d", exitCalled)
	}
}

func TestRunApp_UpdateBranches(t *testing.T) {
	origNewFyneApp := newFyneApp
	origNewCoreApp := newCoreApp
	origSetupTray := setupTrayFunc
	origShowMain := showMainWindow
	origInitLogger := initLogger
	origNewUpdater := newUpdater
	origShowUpdate := showUpdateDialog
	origDelay := updateCheckDelay
	t.Cleanup(func() {
		newFyneApp = origNewFyneApp
		newCoreApp = origNewCoreApp
		setupTrayFunc = origSetupTray
		showMainWindow = origShowMain
		initLogger = origInitLogger
		newUpdater = origNewUpdater
		showUpdateDialog = origShowUpdate
		updateCheckDelay = origDelay
	})

	testApp := fynetest.NewApp()
	testApp.Preferences().SetBool("auto_check_updates", true)
	newFyneApp = func() fyne.App { return testApp }
	dir := t.TempDir()
	newCoreApp = func() (*appcore.App, error) { return appcore.NewAppAt(dir) }
	setupTrayFunc = func(_ fyne.App, _ fyne.Window, _ func(), _ *TrayCallbacks) {}
	showMainWindow = func(*ui.MainWindow) {}
	initLogger = func(string) error { return nil }
	updateCheckDelay = 0

	updateCh := make(chan string, 2)

	newUpdater = func() updateService {
		return &fakeMainUpdater{err: errors.New("check failed")}
	}
	showUpdateDialog = func(_ fyne.Window, _ *updater.ReleaseInfo, _ updateService) {
		updateCh <- "unexpected"
	}
	log.SetOutput(io.Discard)
	if err := runApp(); err != nil {
		t.Fatalf("runApp error branch: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	newUpdater = func() updateService {
		return &fakeMainUpdater{release: &updater.ReleaseInfo{TagName: "v1.2.3"}}
	}
	showUpdateDialog = func(_ fyne.Window, release *updater.ReleaseInfo, _ updateService) {
		updateCh <- release.TagName
	}
	if err := runApp(); err != nil {
		t.Fatalf("runApp update branch: %v", err)
	}

	select {
	case tag := <-updateCh:
		if tag != "v1.2.3" {
			t.Fatalf("unexpected release tag: %q", tag)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected update dialog branch")
	}
}

func TestRunApp_NewCoreAppError(t *testing.T) {
	origNewCoreApp := newCoreApp
	t.Cleanup(func() { newCoreApp = origNewCoreApp })

	newCoreApp = func() (*appcore.App, error) {
		return nil, errors.New("boom")
	}

	err := runApp()
	if err == nil || err.Error() == "" {
		t.Fatal("expected runApp to return wrapped error")
	}
}

func TestMain_UsesInjectedLifecycle(t *testing.T) {
	origEnsure := ensureSingleInstanceFunc
	origSetup := setupConsoleFunc
	origRun := runAppFunc
	origFatal := fatalf
	t.Cleanup(func() {
		ensureSingleInstanceFunc = origEnsure
		setupConsoleFunc = origSetup
		runAppFunc = origRun
		fatalf = origFatal
	})

	ensureCalled := 0
	setupCalled := 0
	runCalled := 0
	fatalCalled := 0

	ensureSingleInstanceFunc = func() { ensureCalled++ }
	setupConsoleFunc = func() { setupCalled++ }
	runAppFunc = func() error {
		runCalled++
		return nil
	}
	fatalf = func(string, ...any) { fatalCalled++ }

	main()

	if ensureCalled != 1 || setupCalled != 1 || runCalled != 1 {
		t.Fatalf("unexpected lifecycle counts: ensure=%d setup=%d run=%d", ensureCalled, setupCalled, runCalled)
	}
	if fatalCalled != 0 {
		t.Fatalf("fatalf should not be called on success, got %d", fatalCalled)
	}
}

func TestMain_FatalOnRunError(t *testing.T) {
	origEnsure := ensureSingleInstanceFunc
	origSetup := setupConsoleFunc
	origRun := runAppFunc
	origFatal := fatalf
	t.Cleanup(func() {
		ensureSingleInstanceFunc = origEnsure
		setupConsoleFunc = origSetup
		runAppFunc = origRun
		fatalf = origFatal
	})

	runAppFunc = func() error { return errors.New("boom") }
	fatalCalled := 0
	fatalf = func(format string, args ...any) {
		fatalCalled++
		if format == "" || len(args) != 1 {
			t.Fatalf("unexpected fatal arguments: %q %v", format, args)
		}
	}

	main()

	if fatalCalled != 1 {
		t.Fatalf("expected fatalf once, got %d", fatalCalled)
	}
}
