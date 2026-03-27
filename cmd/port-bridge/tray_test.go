package main

import (
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

type fakeDesktopApp struct {
	fyne.App
	menu *fyne.Menu
	icon fyne.Resource
}

func (a *fakeDesktopApp) SetSystemTrayMenu(menu *fyne.Menu) {
	a.menu = menu
}

func (a *fakeDesktopApp) SetSystemTrayIcon(icon fyne.Resource) {
	a.icon = icon
}

func (a *fakeDesktopApp) SetSystemTrayWindow(fyne.Window) {}

type fakeTrayWindow struct {
	fyne.Window
	shown   int
	focused int
}

func (w *fakeTrayWindow) Show()         { w.shown++ }
func (w *fakeTrayWindow) RequestFocus() { w.focused++ }

func TestHandleWindowClose_UsesSavedExitAction(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	prefs := app.Preferences()
	prefs.SetBool("close_action_saved", true)
	prefs.SetString("close_action", "exit")

	var minimized, exited int
	handleWindowClose(nil, prefs, func() { minimized++ }, func() { exited++ })

	if minimized != 0 {
		t.Fatalf("minimize should not be called, got %d", minimized)
	}
	if exited != 1 {
		t.Fatalf("exit should be called once, got %d", exited)
	}
}

func TestHandleWindowClose_ShowsDialogWhenNotSaved(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origShowClose := showCloseDialogFn
	t.Cleanup(func() { showCloseDialogFn = origShowClose })

	called := false
	showCloseDialogFn = func(w fyne.Window, prefs fyne.Preferences, onMinimize, onExit func()) {
		called = true
	}

	handleWindowClose(nil, app.Preferences(), func() {}, func() {})

	if !called {
		t.Fatal("expected handleWindowClose to delegate to showCloseDialog when preference is not saved")
	}
}

func TestHandleWindowClose_UsesSavedMinimizeAction(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	prefs := app.Preferences()
	prefs.SetBool("close_action_saved", true)
	prefs.SetString("close_action", "minimize")

	var minimized, exited int
	handleWindowClose(nil, prefs, func() { minimized++ }, func() { exited++ })

	if minimized != 1 {
		t.Fatalf("minimize should be called once, got %d", minimized)
	}
	if exited != 0 {
		t.Fatalf("exit should not be called, got %d", exited)
	}
}

func TestShowCloseDialog_RememberChoiceAndSingleton(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	window := app.NewWindow("test")
	prefs := app.Preferences()

	origFactory := newCustomConfirm
	closeDialogOpen.Store(false)
	t.Cleanup(func() {
		newCustomConfirm = origFactory
		closeDialogOpen.Store(false)
	})

	var callback func(bool)
	var content fyne.CanvasObject
	var callCount atomic.Int32
	newCustomConfirm = func(title, confirm, dismiss string, body fyne.CanvasObject, cb func(bool), parent fyne.Window) *dialog.ConfirmDialog {
		callCount.Add(1)
		content = body
		callback = cb
		return origFactory(title, confirm, dismiss, body, cb, parent)
	}

	var minimized, exited int
	showCloseDialog(window, prefs, func() { minimized++ }, func() { exited++ })
	showCloseDialog(window, prefs, func() { minimized++ }, func() { exited++ })

	if callCount.Load() != 1 {
		t.Fatalf("expected singleton dialog behavior, got %d dialogs", callCount.Load())
	}

	body := content.(*fyne.Container)
	actionGroup := body.Objects[1].(*widget.RadioGroup)
	rememberCheck := body.Objects[2].(*widget.Check)
	actionGroup.SetSelected("Exit program")
	rememberCheck.SetChecked(true)

	callback(true)

	if exited != 1 || minimized != 0 {
		t.Fatalf("expected exit callback only, minimized=%d exited=%d", minimized, exited)
	}
	if !prefs.Bool("close_action_saved") {
		t.Fatal("expected remembered choice to be persisted")
	}
	if got := prefs.StringWithFallback("close_action", ""); got != "exit" {
		t.Fatalf("expected saved action 'exit', got %q", got)
	}
}

func TestRebuildTrayMenu_NoDesktopAppIsNoOp(t *testing.T) {
	trayState.fyneApp = fynetest.NewApp()
	t.Cleanup(trayState.fyneApp.Quit)
	trayState.window = nil
	trayState.onExit = func() {}

	rebuildTrayMenu()
}

func TestSetupTray_NonDesktopAppIsNoOp(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	trayState.fyneApp = nil
	trayState.window = nil
	trayState.onExit = nil

	setupTray(app, window, func() {}, nil)

	if trayState.fyneApp != nil || trayState.window != nil || trayState.onExit != nil {
		t.Fatal("expected non-desktop setupTray to leave tray state untouched")
	}
}

func TestSetupTray_DesktopAppBuildsMenu(t *testing.T) {
	base := fynetest.NewApp()
	t.Cleanup(base.Quit)
	fakeApp := &fakeDesktopApp{App: base}
	window := &fakeTrayWindow{Window: base.NewWindow("test")}

	called := false
	setupTray(fakeApp, window, func() { called = true }, nil)

	if fakeApp.menu == nil {
		t.Fatal("expected tray menu to be installed")
	}
	if fakeApp.icon == nil {
		t.Fatal("expected tray icon to be installed")
	}
	if len(fakeApp.menu.Items) < 2 {
		t.Fatalf("unexpected tray menu items: %+v", fakeApp.menu.Items)
	}

	fakeApp.menu.Items[0].Action()
	if window.shown != 1 || window.focused != 1 {
		t.Fatalf("expected window show/focus from tray action, shown=%d focused=%d", window.shown, window.focused)
	}

	fakeApp.menu.Items[2].Action()
	if !called {
		t.Fatal("expected quit action to invoke callback")
	}

	i18n.SetLanguage("en")
	i18n.NotifyLanguageChange()
	if fakeApp.menu == nil {
		t.Fatal("expected tray menu to rebuild on language change")
	}
}

func TestShowCloseDialog_CancelResetsSingleton(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	origFactory := newCustomConfirm
	closeDialogOpen.Store(false)
	t.Cleanup(func() {
		newCustomConfirm = origFactory
		closeDialogOpen.Store(false)
	})

	var callback func(bool)
	newCustomConfirm = func(title, confirm, dismiss string, body fyne.CanvasObject, cb func(bool), parent fyne.Window) *dialog.ConfirmDialog {
		callback = cb
		return origFactory(title, confirm, dismiss, body, cb, parent)
	}

	showCloseDialog(window, app.Preferences(), func() {}, func() {})
	callback(false)

	if closeDialogOpen.Load() {
		t.Fatal("cancel should reset closeDialogOpen")
	}
}

func TestShowCloseDialog_DefaultMinimizeWithoutRemember(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	origFactory := newCustomConfirm
	closeDialogOpen.Store(false)
	t.Cleanup(func() {
		newCustomConfirm = origFactory
		closeDialogOpen.Store(false)
	})

	var callback func(bool)
	newCustomConfirm = func(title, confirm, dismiss string, body fyne.CanvasObject, cb func(bool), parent fyne.Window) *dialog.ConfirmDialog {
		callback = cb
		return origFactory(title, confirm, dismiss, body, cb, parent)
	}

	minimized := 0
	exited := 0
	showCloseDialog(window, app.Preferences(), func() { minimized++ }, func() { exited++ })
	callback(true)

	if minimized != 1 || exited != 0 {
		t.Fatalf("expected default action to minimize, minimized=%d exited=%d", minimized, exited)
	}
	if app.Preferences().Bool("close_action_saved") {
		t.Fatal("expected remember choice to stay disabled")
	}
}
