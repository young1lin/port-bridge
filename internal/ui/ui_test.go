package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"
	fynetheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	uistyle "github.com/young1lin/port-bridge/internal/ui/theme"
	"github.com/young1lin/port-bridge/internal/ui/views"
	"github.com/young1lin/port-bridge/internal/updater"
	"github.com/young1lin/port-bridge/internal/version"
)

type fakeUpdateChecker struct {
	release *updater.ReleaseInfo
	err     error
}

type fakeDialog struct {
	showCalled bool
	fixed      bool
	centered   bool
	title      string
	content    fyne.CanvasObject
	size       fyne.Size
	onClosed   func()
}

func (d *fakeDialog) SetContent(content fyne.CanvasObject) {
	d.content = content
}

func (d *fakeDialog) Show() {
	d.showCalled = true
}

func (d *fakeDialog) Resize(size fyne.Size) {
	d.size = size
}

func (d *fakeDialog) SetFixedSize(fixed bool) {
	d.fixed = fixed
}

func (d *fakeDialog) CenterOnScreen() {
	d.centered = true
}

func (d *fakeDialog) SetOnClosed(fn func()) {
	d.onClosed = fn
}

func (f *fakeUpdateChecker) CheckForUpdate() (*updater.ReleaseInfo, error) {
	return f.release, f.err
}

func (f *fakeUpdateChecker) CheckForUpdateWithCache(force bool) (*updater.ReleaseInfo, error) {
	return f.release, f.err
}

func (f *fakeUpdateChecker) DownloadAndApply(_ *updater.ReleaseInfo, _ updater.ProgressCallback) error {
	return nil
}

func collectObjects(root fyne.CanvasObject) []fyne.CanvasObject {
	objects := []fyne.CanvasObject{root}
	if c, ok := root.(*fyne.Container); ok {
		for _, obj := range c.Objects {
			objects = append(objects, collectObjects(obj)...)
		}
	}
	if s, ok := root.(*container.Scroll); ok && s.Content != nil {
		objects = append(objects, collectObjects(s.Content)...)
	}
	return objects
}

func TestLoadingOverlayDismissIsIdempotent(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	overlay := NewLoadingOverlay(window, "Title", "Loading", time.Second)
	overlay.Dismiss(ResultSuccess)
	overlay.Dismiss(ResultError)

	if got := overlay.Wait(); got != ResultSuccess {
		t.Fatalf("expected first dismiss result to win, got %v", got)
	}
}

func TestLoadingOverlay_SetTextAndRendererHelpers(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	overlay := NewLoadingOverlay(window, "Title", "Loading", 0)
	overlay.SetText("Updated")
	if overlay.label.Text != "Updated" {
		t.Fatalf("expected updated label text, got %q", overlay.label.Text)
	}

	renderer := overlay.spinner.CreateRenderer()
	if len(renderer.Objects()) != 1 {
		t.Fatalf("expected one spinner renderer object, got %d", len(renderer.Objects()))
	}
	renderer.Destroy()
	overlay.Dismiss(ResultSuccess)
}

func TestRunWithLoading_Success(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	doneCh := make(chan Result, 1)
	RunWithLoading(window, "Title", "Loading", time.Second, func(_ context.Context) error {
		return nil
	}, func(result Result, err error) {
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		doneCh <- result
	})

	select {
	case result := <-doneCh:
		if result != ResultSuccess {
			t.Fatalf("expected success result, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for success callback")
	}
}

func TestRunWithLoading_ErrorAndTimeout(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	errDone := make(chan Result, 1)
	RunWithLoading(window, "Title", "Loading", time.Second, func(_ context.Context) error {
		return errors.New("boom")
	}, func(result Result, err error) {
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
		errDone <- result
	})

	select {
	case result := <-errDone:
		if result != ResultError {
			t.Fatalf("expected error result, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error callback")
	}

	timeoutDone := make(chan Result, 1)
	RunWithLoading(window, "Title", "Loading", 20*time.Millisecond, func(ctx context.Context) error {
		time.Sleep(60 * time.Millisecond)
		return nil
	}, func(result Result, err error) {
		timeoutDone <- result
	})

	select {
	case result := <-timeoutDone:
		if result != ResultTimeout {
			t.Fatalf("expected timeout result, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timeout callback")
	}
}

func TestRunWithLoading_PanicRecovered(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	RunWithLoading(window, "Title", "Loading", 20*time.Millisecond, func(context.Context) error {
		panic("boom")
	}, func(Result, error) {
		t.Fatal("done callback should not run after panic")
	})

	time.Sleep(100 * time.Millisecond)
}

func TestDotSpinnerMakeSVG(t *testing.T) {
	spinner := newDotSpinner()
	t.Cleanup(spinner.Stop)

	res := spinner.makeSVG(24)
	if res == nil {
		t.Fatal("makeSVG should return a resource")
	}
	if res.Name() != "dots_24" {
		t.Fatalf("unexpected resource name: %s", res.Name())
	}

	(&dotSpinnerRenderer{img: spinner.img}).Destroy()
}

func TestParseURL(t *testing.T) {
	if parseURL("https://github.com/young1lin/port-bridge") == nil {
		t.Fatal("expected valid URL to parse")
	}
	if parseURL("://bad url") != nil {
		t.Fatal("expected invalid URL to return nil")
	}
}

func TestNewMainWindowAndSetViews(t *testing.T) {
	i18n.SetLanguage("en")
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())
	if mw.GetWindow() == nil {
		t.Fatal("main window should provide fyne window")
	}
	if mw.versionLabel.Text != version.ShortVersion() {
		t.Fatalf("unexpected version label: %q", mw.versionLabel.Text)
	}

	connView := views.NewConnectionView(app, mw.GetWindow())
	tunnelView := views.NewTunnelView(app, mw.GetWindow())
	mw.SetViews(connView, tunnelView)
	mw.onLanguageChange()

	if mw.connTab.Text == "" || mw.tunnelTab.Text == "" {
		t.Fatal("tab texts should be populated")
	}
}

func TestMainWindow_ShowDialogsAndCloseHook(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())
	mw.showSettingsDialog()
	mw.showAboutDialog()

	mw.SetOnClose(func() {})
	mw.GetWindow().Close()
}

func TestMainWindow_ShowSettingsDialog_UsesDialogWrapperAndUpdatesPrefs(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	app.Preferences().SetString("language", "en")
	app.Preferences().SetString("theme_mode", "dark")
	app.Preferences().SetBool("auto_check_updates", true)

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	origChildWindow := newChildWindow
	t.Cleanup(func() { newChildWindow = origChildWindow })

	dialogState := &fakeDialog{}
	newChildWindow = func(_ fyne.App, title string) childWindowController {
		dialogState.title = title
		return dialogState
	}

	mw.showSettingsDialog()

	if !dialogState.showCalled || dialogState.title != "Preferences" {
		t.Fatalf("unexpected dialog state: %+v", dialogState)
	}
	if !dialogState.fixed || !dialogState.centered {
		t.Fatalf("expected fixed centered preferences child window, got %+v", dialogState)
	}
	if dialogState.size != fyne.NewSize(520, 380) {
		t.Fatalf("expected preferences dialog at 520x380, got %+v", dialogState)
	}

	var selects []*widget.Select
	var check *widget.Check
	for _, obj := range collectObjects(dialogState.content) {
		switch typed := obj.(type) {
		case *widget.Select:
			selects = append(selects, typed)
		case *widget.Check:
			check = typed
		}
	}

	if len(selects) != 3 || check == nil {
		t.Fatalf("expected three selects and one checkbox, got %d selects and check=%v", len(selects), check != nil)
	}

	if app.Preferences().String("language") != "en" {
		t.Fatalf("expected initial language preference to remain en, got %q", app.Preferences().String("language"))
	}
	if app.Preferences().String("theme_mode") != "dark" {
		t.Fatalf("expected initial theme preference to remain dark, got %q", app.Preferences().String("theme_mode"))
	}
	if selects[1].Selected != "Dark" {
		t.Fatalf("expected theme select to use label text, got %q", selects[1].Selected)
	}

	selects[0].SetSelected("中文")
	selects[1].SetSelected("Light")
	check.SetChecked(false)

	if app.Preferences().String("language") != "zh" {
		t.Fatalf("expected language preference zh, got %q", app.Preferences().String("language"))
	}
	if app.Preferences().String("theme_mode") != "light" {
		t.Fatalf("expected theme preference light, got %q", app.Preferences().String("theme_mode"))
	}
	if app.Preferences().BoolWithFallback("auto_check_updates", true) {
		t.Fatal("expected auto update preference to be disabled")
	}
}

func TestMainWindow_ShowAboutDialog_UsesIconBranch(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	app.SetIcon(fyne.NewStaticResource("icon", []byte("icon")))

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	origChildWindow := newChildWindow
	t.Cleanup(func() { newChildWindow = origChildWindow })

	dialogState := &fakeDialog{}
	newChildWindow = func(_ fyne.App, title string) childWindowController {
		dialogState.title = title
		return dialogState
	}

	mw.showAboutDialog()

	if !dialogState.showCalled || dialogState.title != "About PortBridge" {
		t.Fatalf("unexpected about dialog state: %+v", dialogState)
	}
	if !dialogState.fixed || !dialogState.centered {
		t.Fatalf("expected fixed centered about child window, got %+v", dialogState)
	}
	if dialogState.size != fyne.NewSize(520, 300) {
		t.Fatalf("expected about dialog at 520x300, got %+v", dialogState)
	}
	if len(collectObjects(dialogState.content)) == 0 {
		t.Fatal("expected about dialog content to be present")
	}
}

func TestMainWindow_ShowAboutDialog_WithoutIcon(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	origChildWindow := newChildWindow
	t.Cleanup(func() { newChildWindow = origChildWindow })

	dialogState := &fakeDialog{}
	newChildWindow = func(_ fyne.App, title string) childWindowController {
		dialogState.title = title
		return dialogState
	}

	mw.showAboutDialog()

	if !dialogState.showCalled || dialogState.title != "About PortBridge" {
		t.Fatalf("unexpected about dialog state: %+v", dialogState)
	}
	if !dialogState.fixed || !dialogState.centered {
		t.Fatalf("expected fixed centered about child window, got %+v", dialogState)
	}
	if dialogState.size != fyne.NewSize(520, 300) {
		t.Fatalf("expected about dialog at 520x300, got %+v", dialogState)
	}
	if len(collectObjects(dialogState.content)) == 0 {
		t.Fatal("expected about dialog content to be present")
	}
}

func TestMainWindow_SetupMainMenu_UsesWrapperAndActions(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origMenu := setWindowMainMenu
	origChildWindow := newChildWindow
	origUseNativeMainMenu := useNativeMainMenu
	t.Cleanup(func() {
		setWindowMainMenu = origMenu
		newChildWindow = origChildWindow
		useNativeMainMenu = origUseNativeMainMenu
	})
	useNativeMainMenu = func() bool { return true }

	var menus []*fyne.MainMenu
	setWindowMainMenu = func(_ fyne.Window, menu *fyne.MainMenu) {
		menus = append(menus, menu)
	}

	dialogCalls := 0
	newChildWindow = func(_ fyne.App, title string) childWindowController {
		dialogCalls++
		return &fakeDialog{title: title}
	}

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())
	if len(menus) != 1 {
		t.Fatalf("expected initial menu setup once, got %d", len(menus))
	}
	if len(menus[0].Items) != 2 {
		t.Fatalf("expected two top-level menus, got %d", len(menus[0].Items))
	}

	menus[0].Items[0].Items[0].Action()
	menus[0].Items[1].Items[2].Action()
	if dialogCalls != 2 {
		t.Fatalf("expected settings and about actions to show dialogs, got %d", dialogCalls)
	}

	before := len(menus)
	mw.onLanguageChange()
	if len(menus) <= before {
		t.Fatalf("expected language change to rebuild menu, got %d menus before and %d after", before, len(menus))
	}
}

func TestMainWindow_SetupMainMenu_NonMacUsesTopBarAndNoNativeMenu(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origMenu := setWindowMainMenu
	origUseNativeMainMenu := useNativeMainMenu
	t.Cleanup(func() {
		setWindowMainMenu = origMenu
		useNativeMainMenu = origUseNativeMainMenu
	})

	useNativeMainMenu = func() bool { return false }

	var menus []*fyne.MainMenu
	setWindowMainMenu = func(_ fyne.Window, menu *fyne.MainMenu) {
		menus = append(menus, menu)
	}

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	// Non-Mac should use topBar (action bar) since there's no native menu
	if mw.topBar == nil {
		t.Fatal("expected non-mac main window to use top action bar")
	}
	if len(menus) != 1 || menus[0] != nil {
		t.Fatalf("expected one nil native menu assignment, got %+v", menus)
	}

	mw.onLanguageChange()
	if len(menus) != 2 || menus[1] != nil {
		t.Fatalf("expected language refresh to keep native menu nil, got %+v", menus)
	}
}

func TestNewMainWindow_SystemThemeListener(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	app.Preferences().SetString("theme_mode", "system")

	origVariant := currentThemeVariant
	t.Cleanup(func() { currentThemeVariant = origVariant })
	currentThemeVariant = func(fyne.App) fyne.ThemeVariant { return fynetheme.VariantDark }

	appTheme := uistyle.NewPortBridgeTheme()
	appTheme.SetDark(false)
	_ = NewMainWindow(app, appTheme)

	app.Settings().SetTheme(appTheme)
	time.Sleep(50 * time.Millisecond)

	if !appTheme.IsDark() {
		t.Fatal("expected system theme listener to update theme mode")
	}
}

func TestMainWindow_Show_UsesWrapper(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	origShowAndRun := showAndRunWindow
	t.Cleanup(func() { showAndRunWindow = origShowAndRun })

	called := false
	showAndRunWindow = func(window fyne.Window) {
		called = true
		if window != mw.GetWindow() {
			t.Fatalf("unexpected window passed to Show: %v", window)
		}
	}

	mw.Show()

	if !called {
		t.Fatal("expected Show to use wrapper")
	}
}

func TestMainWindow_OnCheckForUpdates_Branches(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	mw := NewMainWindow(app, uistyle.NewPortBridgeTheme())

	origChecker := newUpdateChecker
	origShowAvailable := showUpdateAvailableDialog
	origShowError := showError
	origShowInfo := showInformation
	t.Cleanup(func() {
		newUpdateChecker = origChecker
		showUpdateAvailableDialog = origShowAvailable
		showError = origShowError
		showInformation = origShowInfo
	})

	errCh := make(chan error, 1)
	infoCh := make(chan string, 1)
	availableCh := make(chan string, 1)

	showError = func(err error, _ fyne.Window) { errCh <- err }
	showInformation = func(_, message string, _ fyne.Window) { infoCh <- message }
	showUpdateAvailableDialog = func(_ fyne.Window, release *updater.ReleaseInfo, _ updaterService) {
		availableCh <- release.TagName
	}

	newUpdateChecker = func() updaterService { return &fakeUpdateChecker{err: errors.New("boom")} }
	mw.onCheckForUpdates()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected error branch")
	}

	newUpdateChecker = func() updaterService { return &fakeUpdateChecker{} }
	mw.onCheckForUpdates()
	select {
	case msg := <-infoCh:
		if msg != "You're up to date!" {
			t.Fatalf("unexpected info message: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected up-to-date branch")
	}

	newUpdateChecker = func() updaterService { return &fakeUpdateChecker{release: &updater.ReleaseInfo{TagName: "v1.2.3"}} }
	mw.onCheckForUpdates()
	select {
	case tag := <-availableCh:
		if tag != "v1.2.3" {
			t.Fatalf("unexpected release tag: %q", tag)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected available-update branch")
	}
}
