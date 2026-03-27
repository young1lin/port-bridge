package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

// closeDialogOpen prevents opening multiple close dialogs when X is clicked rapidly
var closeDialogOpen atomic.Bool

// trayState holds references needed to rebuild the tray menu on language change.
var trayState struct {
	mu                  sync.RWMutex
	fyneApp             fyne.App
	window              fyne.Window
	onExit              func()
	getRunningCount     func() int
	startAllTunnels     func()
	stopAllTunnels      func()
	getTotalTunnelCount func() int
}

var (
	newCustomConfirm  = dialog.NewCustomConfirm
	showCloseDialogFn = showCloseDialog
)

// TrayCallbacks contains callbacks for tray menu actions
type TrayCallbacks struct {
	GetRunningCount     func() int
	GetTotalTunnelCount func() int
	StartAllTunnels     func()
	StopAllTunnels      func()
}

// setupTray initializes the system tray icon and right-click menu.
func setupTray(fyneApp fyne.App, window fyne.Window, onExit func(), callbacks *TrayCallbacks) {
	_, ok := fyneApp.(desktop.App)
	if !ok {
		log.Println("[WARN] System tray not supported on this platform")
		return
	}

	trayState.mu.Lock()
	trayState.fyneApp = fyneApp
	trayState.window = window
	trayState.onExit = onExit

	if callbacks != nil {
		trayState.getRunningCount = callbacks.GetRunningCount
		trayState.getTotalTunnelCount = callbacks.GetTotalTunnelCount
		trayState.startAllTunnels = callbacks.StartAllTunnels
		trayState.stopAllTunnels = callbacks.StopAllTunnels
	}
	trayState.mu.Unlock()

	rebuildTrayMenu()

	// Register language change callback to rebuild tray menu
	i18n.OnLanguageChange(rebuildTrayMenu)

	log.Println("[DEBUG] System tray initialized")
}

// rebuildTrayMenu rebuilds the system tray menu with the current language.
func rebuildTrayMenu() {
	trayState.mu.RLock()
	fyneApp := trayState.fyneApp
	window := trayState.window
	onExit := trayState.onExit
	getRunningCount := trayState.getRunningCount
	getTotalTunnelCount := trayState.getTotalTunnelCount
	startAllTunnels := trayState.startAllTunnels
	stopAllTunnels := trayState.stopAllTunnels
	trayState.mu.RUnlock()

	desk, ok := fyneApp.(desktop.App)
	if !ok {
		return
	}

	var items []*fyne.MenuItem

	// Show Window
	items = append(items, fyne.NewMenuItem(i18n.L("Show Window"), func() {
		window.Show()
		window.RequestFocus()
	}))

	// Tunnel status and controls (if callbacks are available)
	if getRunningCount != nil && getTotalTunnelCount != nil {
		items = append(items, fyne.NewMenuItemSeparator())

		running := getRunningCount()
		total := getTotalTunnelCount()
		statusText := fmt.Sprintf(i18n.L("Tunnels: %d/%d running"), running, total)

		// Status item (disabled, just for display)
		statusItem := fyne.NewMenuItem(statusText, nil)
		statusItem.Disabled = true
		items = append(items, statusItem)

		// Start All / Stop All
		if startAllTunnels != nil && running < total {
			items = append(items, fyne.NewMenuItem(i18n.L("Start All Tunnels"), func() {
				go startAllTunnels()
			}))
		}
		if stopAllTunnels != nil && running > 0 {
			items = append(items, fyne.NewMenuItem(i18n.L("Stop All Tunnels"), func() {
				go stopAllTunnels()
			}))
		}
	}

	items = append(items, fyne.NewMenuItemSeparator())

	// Quit
	quitItem := fyne.NewMenuItem(i18n.L("Quit"), onExit)
	quitItem.IsQuit = true
	items = append(items, quitItem)

	menu := fyne.NewMenu("", items...)
	desk.SetSystemTrayMenu(menu)
	desk.SetSystemTrayIcon(appIcon)
}

// RefreshTrayMenu refreshes the tray menu to update tunnel status
func RefreshTrayMenu() {
	rebuildTrayMenu()
}

// handleWindowClose is called when the user clicks the window close (X) button.
// If the user previously chose "remember", the saved action is applied immediately.
// Otherwise a dialog is shown to choose minimize-to-tray or exit.
func handleWindowClose(w fyne.Window, prefs fyne.Preferences, onMinimize, onExit func()) {
	if prefs.Bool("close_action_saved") {
		if prefs.StringWithFallback("close_action", "minimize") == "exit" {
			onExit()
		} else {
			onMinimize()
		}
		return
	}
	showCloseDialogFn(w, prefs, onMinimize, onExit)
}

// showCloseDialog shows a dialog asking what to do when the window is closed.
// It is a singleton: a second call while the dialog is open is a no-op.
func showCloseDialog(w fyne.Window, prefs fyne.Preferences, onMinimize, onExit func()) {
	if !closeDialogOpen.CompareAndSwap(false, true) {
		return // dialog is already showing
	}

	// Use internal action constants (not user-facing strings)
	const (
		actionMinimize = "minimize"
		actionExit     = "exit"
	)

	optMinimize := i18n.L("Minimize to system tray")
	optExit := i18n.L("Exit program")

	actionGroup := widget.NewRadioGroup([]string{optMinimize, optExit}, nil)
	actionGroup.SetSelected(optMinimize)

	rememberCheck := widget.NewCheck(i18n.L("Remember my choice"), nil)

	content := container.NewVBox(
		widget.NewLabel(i18n.L("Please choose the action when closing the window:")),
		actionGroup,
		rememberCheck,
	)

	dlg := newCustomConfirm(i18n.L("Close Window"), i18n.L("OK"), i18n.L("Cancel"), content, func(confirmed bool) {
		closeDialogOpen.Store(false) // allow dialog to open again next time
		if !confirmed {
			return // user cancelled, window stays visible
		}
		if rememberCheck.Checked {
			action := actionMinimize
			if actionGroup.Selected == optExit {
				action = actionExit
			}
			prefs.SetString("close_action", action)
			prefs.SetBool("close_action_saved", true)
		}
		if actionGroup.Selected == optExit {
			onExit()
		} else {
			onMinimize()
		}
	}, w)
	dlg.Show()
}
