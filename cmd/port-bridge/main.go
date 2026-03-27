package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynetheme "fyne.io/fyne/v2/theme"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/logger"
	"github.com/young1lin/port-bridge/internal/presenter"
	"github.com/young1lin/port-bridge/internal/ui"
	"github.com/young1lin/port-bridge/internal/ui/dialogs"
	"github.com/young1lin/port-bridge/internal/ui/theme"
	"github.com/young1lin/port-bridge/internal/ui/views"
	"github.com/young1lin/port-bridge/internal/updater"
	"github.com/young1lin/port-bridge/internal/version"
)

//go:embed translations/*
var translationsFS embed.FS

type updateService interface {
	CheckForUpdate() (*updater.ReleaseInfo, error)
	DownloadAndApply(release *updater.ReleaseInfo, progress updater.ProgressCallback) error
}

var (
	ensureSingleInstanceFunc = ensureSingleInstance
	setupConsoleFunc         = setupConsole
	runAppFunc               = runApp
	fatalf                   = log.Fatalf
	newFyneApp               = func() fyne.App { return app.NewWithID("com.portbridge") }
	addTranslationsFS        = i18n.AddTranslationsFS
	newMainWindow            = ui.NewMainWindow
	newConnectionView        = views.NewConnectionView
	newTunnelView            = views.NewTunnelView
	newCoreApp               = appcore.NewApp
	setupTrayFunc            = setupTray
	handleCloseFunc          = handleWindowClose
	setMainWindowClose       = func(mainWindow *ui.MainWindow, callback func()) { mainWindow.SetOnClose(callback) }
	showMainWindow           = func(mainWindow *ui.MainWindow) { mainWindow.Show() }
	newUpdater               = func() updateService { return updater.NewUpdater() }
	updateCheckDelay         = 3 * time.Second
	initLogger               = logger.Init
	showUpdateDialog         = func(win fyne.Window, release *updater.ReleaseInfo, u updateService) {
		dialogs.ShowUpdateAvailableDialog(win, release, u)
	}
)

func main() {
	// Prevent multiple instances from running simultaneously
	ensureSingleInstanceFunc()

	// Fix console encoding for current platform
	setupConsoleFunc()

	if err := runAppFunc(); err != nil {
		fatalf("[ERROR] Failed to initialize application: %v", err)
	}
}

func runApp() error {
	// Initialize rotating logger
	logDir := filepath.Join(os.Getenv("APPDATA"), "portbridge", "logs")
	if err := initLogger(logDir); err != nil {
		log.Printf("[WARN] Failed to initialize log file: %v", err)
	}

	log.Printf("[DEBUG] Starting SSH Port Forward Tool v%s", version.Version)
	log.Printf("[DEBUG] Log file: %s", logger.GetLogPath())

	// Initialize application core
	log.Println("[DEBUG] Initializing application...")
	application, err := newCoreApp()
	if err != nil {
		return fmt.Errorf("new app: %w", err)
	}
	defer func() {
		log.Println("[DEBUG] Shutting down application...")
		application.Shutdown()
	}()

	// Initialize Fyne app and UI
	fyneApp, mainWindow, connView, tunnelView := setupUI()

	// Create presenters
	log.Println("[DEBUG] Creating presenters...")
	connPresenter := presenter.NewConnectionPresenter(application, fyneApp, mainWindow.GetWindow())
	tunnelPresenter := presenter.NewTunnelPresenter(application, fyneApp, mainWindow.GetWindow())

	// Bind presenters to views
	connPresenter.SetView(connView)
	tunnelPresenter.SetView(tunnelView)

	// Subscribe to tunnel status changes
	log.Println("[DEBUG] Subscribing to tunnel status changes...")
	application.GetTunnelManager().AddStatusCallback(tunnelPresenter.OnStatusChange)

	// Load initial data
	log.Println("[DEBUG] Loading initial data...")
	connPresenter.RefreshData()
	tunnelPresenter.RefreshData()
	log.Printf("[DEBUG] Loaded %d connections, %d tunnels",
		len(application.GetStore().GetConnections()),
		len(application.GetStore().GetTunnels()))

	// doExit performs a clean shutdown and quits the Fyne event loop
	doExit := func() {
		log.Println("[DEBUG] Exit requested")
		application.Shutdown()
		fyneApp.Quit()
	}

	// Setup system tray
	trayCallbacks := &TrayCallbacks{
		GetRunningCount: func() int {
			return application.GetTunnelManager().GetRunningCount()
		},
		GetTotalTunnelCount: func() int {
			return len(application.GetStore().GetTunnels())
		},
		StartAllTunnels: func() {
			tunnels := application.GetStore().GetTunnels()
			for _, t := range tunnels {
				if !application.GetTunnelManager().IsRunning(t.ID) {
					application.GetTunnelManager().StartTunnel(t.ID)
				}
			}
		},
		StopAllTunnels: func() {
			application.GetTunnelManager().StopAll()
		},
	}
	setupTrayFunc(fyneApp, mainWindow.GetWindow(), doExit, trayCallbacks)

	// Handle window close button (X): show dialog or apply remembered choice
	setMainWindowClose(mainWindow, func() {
		log.Println("[DEBUG] Window close requested")
		handleCloseFunc(
			mainWindow.GetWindow(),
			fyneApp.Preferences(),
			func() { mainWindow.GetWindow().Hide() },
			doExit,
		)
	})

	// Show and run
	log.Println("[DEBUG] Showing main window...")
	showMainWindow(mainWindow)

	// Auto-check for updates (delayed, in background)
	if fyneApp.Preferences().BoolWithFallback("auto_check_updates", true) {
		go func() {
			time.Sleep(updateCheckDelay) // delay to avoid blocking startup
			win := mainWindow.GetWindow()
			u := newUpdater()
			release, err := u.CheckForUpdate()
			if err != nil {
				log.Printf("[WARN] Update check failed: %v", err)
				return
			}
			if release == nil {
				return
			}
			fyne.Do(func() {
				showUpdateDialog(win, release, u)
			})
		}()
	}

	return nil
}

// setupUI initializes Fyne app and creates UI components
func setupUI() (fyne.App, *ui.MainWindow, *views.ConnectionView, *views.TunnelView) {
	log.Println("[DEBUG] Creating Fyne app...")
	fyneApp := newFyneApp()
	fyneApp.SetIcon(appIcon)

	// Load translations
	log.Println("[DEBUG] Loading translations...")
	if err := addTranslationsFS(translationsFS, "translations"); err != nil {
		log.Printf("[WARN] Failed to load translations: %v", err)
	}

	// Restore saved language preference, default to English
	savedLang := fyneApp.Preferences().StringWithFallback("language", "en")
	i18n.SetLanguage(savedLang)

	// Set theme (default follows system, restore saved preference)
	log.Println("[DEBUG] Setting up theme...")
	appTheme := theme.NewPortBridgeTheme()
	switch fyneApp.Preferences().StringWithFallback("theme_mode", "system") {
	case "dark":
		appTheme.SetDark(true)
	case "light":
		appTheme.SetDark(false)
	default: // "system"
		appTheme.SetDark(fyneApp.Settings().ThemeVariant() == fynetheme.VariantDark)
	}
	fyneApp.Settings().SetTheme(appTheme)

	// Create main window
	log.Println("[DEBUG] Creating main window...")
	mainWindow := newMainWindow(fyneApp, appTheme)
	window := mainWindow.GetWindow()

	// Create views
	log.Println("[DEBUG] Creating views...")
	connView := newConnectionView(fyneApp, window)
	tunnelView := newTunnelView(fyneApp, window)

	// Set views
	mainWindow.SetViews(connView, tunnelView)

	return fyneApp, mainWindow, connView, tunnelView
}
