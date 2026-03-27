package presenter

import (
	"context"
	"log"
	"time"

	"fyne.io/fyne/v2"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ui"
	"github.com/young1lin/port-bridge/internal/ui/views"
)

// ConnectionPresenter handles the connection view logic
type ConnectionPresenter struct {
	app      *appcore.App
	services connectionServices
	fyneApp  fyne.App
	window   fyne.Window
	view     connectionView
}

// NewConnectionPresenter creates a new connection presenter
func NewConnectionPresenter(app *appcore.App, fyneApp fyne.App, window fyne.Window) *ConnectionPresenter {
	return &ConnectionPresenter{
		app:      app,
		services: newConnectionServices(app),
		fyneApp:  fyneApp,
		window:   window,
	}
}

// SetView binds the view and sets up callbacks
func (p *ConnectionPresenter) SetView(view connectionView) {
	p.view = view
	view.SetOnEdit(p.OnEdit)
	view.SetOnDelete(p.OnDelete)
	view.SetOnTest(p.OnTest)
}

// RefreshData refreshes the connection list from model.
// Safe to call from any goroutine — UI updates are always dispatched to the main thread.
func (p *ConnectionPresenter) RefreshData() {
	connections := p.services.store.GetConnections()
	log.Printf("[DEBUG] Refreshing connections: count=%d", len(connections))

	items := make([]views.ConnectionItem, len(connections))
	for i, conn := range connections {
		isConnected := p.services.clientManager.IsConnected(conn.ID)
		statusIcon := "⚪"
		if isConnected {
			statusIcon = "🟢"
		}
		items[i] = views.ConnectionItem{
			ID:          conn.ID,
			Name:        conn.Name,
			Address:     conn.Address(),
			IsConnected: isConnected,
			StatusIcon:  statusIcon,
		}
	}
	fyneDo(func() {
		p.view.SetData(items)
	})
}

// OnEdit handles edit/new connection action
func (p *ConnectionPresenter) OnEdit(id string) {
	log.Printf("[DEBUG] Edit connection: id=%s", id)

	var conn *models.SSHConnection
	if id != "" {
		conn = p.services.store.GetConnection(id)
	}

	dlg := newConnectionDialog(p.fyneApp, p.window, conn)
	dlg.SetOnSave(func(saved *models.SSHConnection) {
		log.Printf("[DEBUG] Saving connection: id=%s, name=%s", saved.ID, saved.Name)
		if err := p.services.store.SaveConnection(saved); err != nil {
			log.Printf("[ERROR] Failed to save connection: %v", err)
			showError(err, p.window)
			return
		}
		p.RefreshData()
	})
	dlg.Show()
}

// OnDelete handles delete connection action
func (p *ConnectionPresenter) OnDelete(id string) {
	log.Printf("[DEBUG] Delete connection: id=%s", id)

	p.view.ShowConfirm(i18n.L("Confirm Delete"), i18n.L("Are you sure you want to delete this connection?"), func(confirmed bool) {
		if confirmed {
			if err := p.services.store.DeleteConnection(id); err != nil {
				showError(err, p.window)
				return
			}
			p.RefreshData()
		}
	})
}

// OnTest handles test connection action
func (p *ConnectionPresenter) OnTest(id string) {
	log.Printf("[DEBUG] Test connection: id=%s", id)

	conn := p.services.store.GetConnection(id)
	if conn == nil {
		return
	}

	runWithLoading(p.window, i18n.L("Test Connection"), i18n.L("Connecting..."), 5*time.Second, func(ctx context.Context) error {
		client := newSSHTestClient(conn)
		return testSSHConnection(client)
	}, func(result ui.Result, err error) {
		fyneDo(func() {
			if err != nil {
				showError(MakeFriendlyError(err), p.window)
			} else {
				showInformation(i18n.L("Test Succeeded"), i18n.L("Connection test succeeded!"), p.window)
			}
		})
	})
}
