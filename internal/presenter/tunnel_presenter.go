package presenter

import (
	"log"

	"fyne.io/fyne/v2"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ui/views"
)

// TunnelPresenter handles the tunnel view logic
type TunnelPresenter struct {
	app      *appcore.App
	services tunnelServices
	fyneApp  fyne.App
	window   fyne.Window
	view     tunnelView
}

// NewTunnelPresenter creates a new tunnel presenter
func NewTunnelPresenter(app *appcore.App, fyneApp fyne.App, window fyne.Window) *TunnelPresenter {
	return &TunnelPresenter{
		app:      app,
		services: newTunnelServices(app),
		fyneApp:  fyneApp,
		window:   window,
	}
}

// SetView binds the view and sets up callbacks
func (p *TunnelPresenter) SetView(view tunnelView) {
	p.view = view
	view.SetOnEdit(p.OnEdit)
	view.SetOnDelete(p.OnDelete)
	view.SetOnStart(p.OnStart)
	view.SetOnStop(p.OnStop)
	view.SetOnStartAll(p.OnStartAll)
	view.SetOnStopAll(p.OnStopAll)
}

// RefreshData refreshes the tunnel list from model.
// Safe to call from any goroutine — UI updates are always dispatched to the main thread.
func (p *TunnelPresenter) RefreshData() {
	tunnels := p.services.store.GetTunnels()
	log.Printf("[DEBUG] Refreshing tunnels: count=%d", len(tunnels))

	items := make([]views.TunnelItem, len(tunnels))
	for i, tunnel := range tunnels {
		status := p.services.manager.GetStatus(tunnel.ID)
		isRunning := p.services.manager.IsRunning(tunnel.ID)

		items[i] = views.TunnelItem{
			ID:          tunnel.ID,
			Name:        tunnel.Name,
			LocalPort:   tunnel.LocalPort,
			Target:      tunnel.TargetAddress(),
			Status:      i18n.StatusText(status.String()),
			StatusColor: status.Color(),
			IsRunning:   isRunning,
		}
	}
	fyneDo(func() {
		p.view.SetData(items)
	})
}

// OnEdit handles edit/new tunnel action
func (p *TunnelPresenter) OnEdit(id string) {
	log.Printf("[DEBUG] Edit tunnel: id=%s", id)

	var tunnel *models.Tunnel
	if id != "" {
		tunnel = p.services.store.GetTunnel(id)
	}

	connections := p.services.store.GetConnections()
	dlg := newTunnelDialog(p.fyneApp, p.window, tunnel, connections)
	dlg.SetOnSave(func(saved *models.Tunnel) {
		log.Printf("[DEBUG] Saving tunnel: id=%s, name=%s", saved.ID, saved.Name)
		if err := p.services.store.SaveTunnel(saved); err != nil {
			showError(err, p.window)
			return
		}
		p.RefreshData()
	})
	dlg.Show()
}

// OnDelete handles delete tunnel action
func (p *TunnelPresenter) OnDelete(id string) {
	log.Printf("[DEBUG] Delete tunnel: id=%s", id)

	p.view.ShowConfirm(i18n.L("Confirm Delete"), i18n.L("Are you sure you want to delete this forwarding rule?"), func(confirmed bool) {
		if confirmed {
			go func() {
				p.services.manager.StopTunnel(id)
				if err := p.services.store.DeleteTunnel(id); err != nil {
					fyneDo(func() {
						showError(err, p.window)
					})
					return
				}
				p.RefreshData()
			}()
		}
	})
}

// OnStart handles start tunnel action
func (p *TunnelPresenter) OnStart(id string) {
	log.Printf("[DEBUG] Start tunnel: id=%s", id)

	go func() {
		if err := p.services.manager.StartTunnel(id); err != nil {
			fyneDo(func() {
				showError(MakeFriendlyError(err), p.window)
			})
		}
		p.RefreshData()
	}()
}

// OnStop handles stop tunnel action
func (p *TunnelPresenter) OnStop(id string) {
	log.Printf("[DEBUG] Stop tunnel: id=%s", id)

	go func() {
		p.services.manager.StopTunnel(id)
		p.RefreshData()
	}()
}

// OnStartAll handles start all tunnels action
func (p *TunnelPresenter) OnStartAll() {
	log.Println("[DEBUG] Start all tunnels")

	go func() {
		tunnels := p.services.store.GetTunnels()
		for _, tunnel := range tunnels {
			if !p.services.manager.IsRunning(tunnel.ID) {
				if err := p.services.manager.StartTunnel(tunnel.ID); err != nil {
					log.Printf("[ERROR] Failed to start tunnel %s: %v", tunnel.ID, err)
				}
			}
		}
		p.RefreshData()
	}()
}

// OnStopAll handles stop all tunnels action
func (p *TunnelPresenter) OnStopAll() {
	log.Println("[DEBUG] Stop all tunnels")

	go func() {
		p.services.manager.StopAll()
		p.RefreshData()
	}()
}

// OnStatusChange handles tunnel status change callback
func (p *TunnelPresenter) OnStatusChange(tunnelID string, status models.TunnelStatus, err error) {
	log.Printf("[DEBUG] Tunnel status: id=%s, status=%s", tunnelID, status.String())
	p.RefreshData()
}
