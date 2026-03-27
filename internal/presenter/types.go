package presenter

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ssh"
	"github.com/young1lin/port-bridge/internal/ui"
	"github.com/young1lin/port-bridge/internal/ui/dialogs"
	"github.com/young1lin/port-bridge/internal/ui/views"
)

type connectionStore interface {
	GetConnections() []*models.SSHConnection
	GetConnection(id string) *models.SSHConnection
	SaveConnection(conn *models.SSHConnection) error
	DeleteConnection(id string) error
}

type connectionClientManager interface {
	IsConnected(connID string) bool
}

type tunnelStore interface {
	GetTunnels() []*models.Tunnel
	GetTunnel(id string) *models.Tunnel
	SaveTunnel(tunnel *models.Tunnel) error
	DeleteTunnel(id string) error
	GetConnections() []*models.SSHConnection
}

type tunnelManager interface {
	GetStatus(tunnelID string) models.TunnelStatus
	IsRunning(tunnelID string) bool
	StartTunnel(tunnelID string) error
	StopTunnel(tunnelID string) error
	StopAll()
}

type connectionView interface {
	SetOnEdit(callback func(id string))
	SetOnDelete(callback func(id string))
	SetOnTest(callback func(id string))
	SetData(data []views.ConnectionItem)
	ShowConfirm(title, message string, callback func(bool))
}

type tunnelView interface {
	SetOnEdit(callback func(id string))
	SetOnDelete(callback func(id string))
	SetOnStart(callback func(id string))
	SetOnStop(callback func(id string))
	SetOnStartAll(callback func())
	SetOnStopAll(callback func())
	SetData(data []views.TunnelItem)
	ShowConfirm(title, message string, callback func(bool))
}

type connectionDialog interface {
	SetOnSave(callback func(*models.SSHConnection))
	Show()
}

type tunnelDialog interface {
	SetOnSave(callback func(*models.Tunnel))
	Show()
}

type connectionServices struct {
	store         connectionStore
	clientManager connectionClientManager
}

type tunnelServices struct {
	store   tunnelStore
	manager tunnelManager
}

func newConnectionServices(app *appcore.App) connectionServices {
	return connectionServices{
		store:         app.GetStore(),
		clientManager: app.GetClientManager(),
	}
}

func newTunnelServices(app *appcore.App) tunnelServices {
	return tunnelServices{
		store:   app.GetStore(),
		manager: app.GetTunnelManager(),
	}
}

var (
	newConnectionDialog = func(app fyne.App, window fyne.Window, conn *models.SSHConnection) connectionDialog {
		return dialogs.NewConnectionDialog(app, window, conn)
	}
	newTunnelDialog = func(app fyne.App, window fyne.Window, tunnel *models.Tunnel, connections []*models.SSHConnection) tunnelDialog {
		return dialogs.NewTunnelDialog(app, window, tunnel, connections)
	}
	newSSHTestClient  = ssh.NewClient
	testSSHConnection = func(client *ssh.Client) error {
		return client.TestConnection()
	}
	runWithLoading = func(w fyne.Window, title, text string, timeout time.Duration, task func(ctx context.Context) error, done func(ui.Result, error)) {
		ui.RunWithLoading(w, title, text, timeout, task, done)
	}
	fyneDo    = fyne.Do
	showError = func(err error, window fyne.Window) {
		dialog.ShowError(err, window)
	}
	showInformation = func(title, message string, window fyne.Window) {
		dialog.ShowInformation(title, message, window)
	}
)
