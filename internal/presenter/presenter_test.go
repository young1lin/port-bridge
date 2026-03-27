package presenter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"

	appcore "github.com/young1lin/port-bridge/internal/app"
	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ssh"
	"github.com/young1lin/port-bridge/internal/ui"
	"github.com/young1lin/port-bridge/internal/ui/views"
)

type mockConnectionStore struct {
	connections []*models.SSHConnection
	byID        map[string]*models.SSHConnection
	deleteID    string
	saveConn    *models.SSHConnection
	deleteErr   error
	saveErr     error
}

func (s *mockConnectionStore) GetConnections() []*models.SSHConnection { return s.connections }
func (s *mockConnectionStore) GetConnection(id string) *models.SSHConnection {
	return s.byID[id]
}
func (s *mockConnectionStore) SaveConnection(conn *models.SSHConnection) error {
	s.saveConn = conn
	if s.byID == nil {
		s.byID = map[string]*models.SSHConnection{}
	}
	s.byID[conn.ID] = conn

	updated := false
	for i, existing := range s.connections {
		if existing.ID == conn.ID {
			s.connections[i] = conn
			updated = true
			break
		}
	}
	if !updated {
		s.connections = append(s.connections, conn)
	}
	return s.saveErr
}
func (s *mockConnectionStore) DeleteConnection(id string) error {
	s.deleteID = id
	return s.deleteErr
}

type mockConnectionClientManager struct {
	connected map[string]bool
}

func (m *mockConnectionClientManager) IsConnected(connID string) bool {
	return m.connected[connID]
}

type mockTunnelStore struct {
	tunnels     []*models.Tunnel
	tunnelsByID map[string]*models.Tunnel
	connections []*models.SSHConnection
	deletedID   string
	savedTunnel *models.Tunnel
	deleteErr   error
	saveErr     error
}

func (s *mockTunnelStore) GetTunnels() []*models.Tunnel { return s.tunnels }
func (s *mockTunnelStore) GetTunnel(id string) *models.Tunnel {
	return s.tunnelsByID[id]
}
func (s *mockTunnelStore) SaveTunnel(tunnel *models.Tunnel) error {
	s.savedTunnel = tunnel
	if s.tunnelsByID == nil {
		s.tunnelsByID = map[string]*models.Tunnel{}
	}
	s.tunnelsByID[tunnel.ID] = tunnel

	updated := false
	for i, existing := range s.tunnels {
		if existing.ID == tunnel.ID {
			s.tunnels[i] = tunnel
			updated = true
			break
		}
	}
	if !updated {
		s.tunnels = append(s.tunnels, tunnel)
	}
	return s.saveErr
}
func (s *mockTunnelStore) DeleteTunnel(id string) error {
	s.deletedID = id
	return s.deleteErr
}
func (s *mockTunnelStore) GetConnections() []*models.SSHConnection { return s.connections }

type mockTunnelManager struct {
	statuses      map[string]models.TunnelStatus
	running       map[string]bool
	startedIDs    []string
	stoppedIDs    []string
	stopAllCalled bool
	startErrs     map[string]error
}

func (m *mockTunnelManager) GetStatus(tunnelID string) models.TunnelStatus {
	return m.statuses[tunnelID]
}
func (m *mockTunnelManager) IsRunning(tunnelID string) bool {
	return m.running[tunnelID]
}
func (m *mockTunnelManager) StartTunnel(tunnelID string) error {
	m.startedIDs = append(m.startedIDs, tunnelID)
	return m.startErrs[tunnelID]
}
func (m *mockTunnelManager) StopTunnel(tunnelID string) error {
	m.stoppedIDs = append(m.stoppedIDs, tunnelID)
	return nil
}
func (m *mockTunnelManager) StopAll() {
	m.stopAllCalled = true
}

type mockConnectionView struct {
	data             []views.ConnectionItem
	onEdit           func(id string)
	onDelete         func(id string)
	onTest           func(id string)
	confirmResult    bool
	lastConfirmTitle string
	lastConfirmMsg   string
}

func (v *mockConnectionView) SetOnEdit(callback func(id string))   { v.onEdit = callback }
func (v *mockConnectionView) SetOnDelete(callback func(id string)) { v.onDelete = callback }
func (v *mockConnectionView) SetOnTest(callback func(id string))   { v.onTest = callback }
func (v *mockConnectionView) SetData(data []views.ConnectionItem)  { v.data = data }
func (v *mockConnectionView) ShowConfirm(title, message string, callback func(bool)) {
	v.lastConfirmTitle = title
	v.lastConfirmMsg = message
	callback(v.confirmResult)
}

type mockConnectionDialog struct {
	showCalled bool
	onSave     func(*models.SSHConnection)
}

func (d *mockConnectionDialog) SetOnSave(callback func(*models.SSHConnection)) { d.onSave = callback }
func (d *mockConnectionDialog) Show()                                          { d.showCalled = true }

type mockTunnelView struct {
	data             []views.TunnelItem
	onEdit           func(id string)
	onDelete         func(id string)
	onStart          func(id string)
	onStop           func(id string)
	onStartAll       func()
	onStopAll        func()
	confirmResult    bool
	lastConfirmTitle string
	lastConfirmMsg   string
}

type mockTunnelDialog struct {
	showCalled bool
	onSave     func(*models.Tunnel)
}

func (d *mockTunnelDialog) SetOnSave(callback func(*models.Tunnel)) { d.onSave = callback }
func (d *mockTunnelDialog) Show()                                   { d.showCalled = true }

func (v *mockTunnelView) SetOnEdit(callback func(id string))   { v.onEdit = callback }
func (v *mockTunnelView) SetOnDelete(callback func(id string)) { v.onDelete = callback }
func (v *mockTunnelView) SetOnStart(callback func(id string))  { v.onStart = callback }
func (v *mockTunnelView) SetOnStop(callback func(id string))   { v.onStop = callback }
func (v *mockTunnelView) SetOnStartAll(callback func())        { v.onStartAll = callback }
func (v *mockTunnelView) SetOnStopAll(callback func())         { v.onStopAll = callback }
func (v *mockTunnelView) SetData(data []views.TunnelItem)      { v.data = data }
func (v *mockTunnelView) ShowConfirm(title, message string, callback func(bool)) {
	v.lastConfirmTitle = title
	v.lastConfirmMsg = message
	callback(v.confirmResult)
}

func TestConnectionPresenter_RefreshData(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	store := &mockConnectionStore{
		connections: []*models.SSHConnection{
			{ID: "c1", Name: "Alpha", Host: "host1", Port: 22},
			{ID: "c2", Name: "Beta", Host: "host2", Port: 2222},
		},
	}
	view := &mockConnectionView{}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{connected: map[string]bool{"c1": true}},
		},
		view: view,
	}

	p.RefreshData()

	if len(view.data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(view.data))
	}
	if !view.data[0].IsConnected || view.data[0].StatusIcon != "🟢" {
		t.Fatalf("expected first connection to be connected, got %+v", view.data[0])
	}
	if view.data[1].IsConnected || view.data[1].StatusIcon != "⚪" {
		t.Fatalf("expected second connection to be disconnected, got %+v", view.data[1])
	}
}

func TestNewConnectionPresenter_AndSetView(t *testing.T) {
	core, err := appcore.NewAppAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppAt: %v", err)
	}
	t.Cleanup(core.Shutdown)

	fyneApp := fynetest.NewApp()
	t.Cleanup(fyneApp.Quit)
	window := fyneApp.NewWindow("test")
	view := &mockConnectionView{}

	p := NewConnectionPresenter(core, fyneApp, window)
	p.SetView(view)

	if p.services.store == nil || p.services.clientManager == nil {
		t.Fatal("expected presenter services to be initialized")
	}
	if view.onEdit == nil || view.onDelete == nil || view.onTest == nil {
		t.Fatal("expected view callbacks to be wired")
	}
}

func TestConnectionPresenter_OnDeleteConfirmed(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	store := &mockConnectionStore{}
	view := &mockConnectionView{confirmResult: true}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{},
		},
		view: view,
	}

	p.OnDelete("c1")

	if store.deleteID != "c1" {
		t.Fatalf("expected deleted id c1, got %q", store.deleteID)
	}
}

func TestConnectionPresenter_OnDeleteError(t *testing.T) {
	origShowError := showError
	t.Cleanup(func() { showError = origShowError })

	var shown error
	showError = func(err error, _ fyne.Window) { shown = err }

	store := &mockConnectionStore{deleteErr: errors.New("delete failed")}
	view := &mockConnectionView{confirmResult: true}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{},
		},
		view: view,
	}

	p.OnDelete("c1")

	if shown == nil || shown.Error() != "delete failed" {
		t.Fatalf("expected delete error dialog, got %v", shown)
	}
}

func TestConnectionPresenter_OnEdit_SaveSuccess(t *testing.T) {
	origFactory := newConnectionDialog
	origDo := fyneDo
	t.Cleanup(func() {
		newConnectionDialog = origFactory
		fyneDo = origDo
	})
	fyneDo = func(fn func()) { fn() }

	dialog := &mockConnectionDialog{}
	newConnectionDialog = func(_ fyne.App, _ fyne.Window, conn *models.SSHConnection) connectionDialog {
		if conn == nil {
			t.Fatal("expected existing connection to be passed to dialog")
		}
		return dialog
	}

	store := &mockConnectionStore{
		byID: map[string]*models.SSHConnection{
			"c1": {ID: "c1", Name: "Old"},
		},
	}
	view := &mockConnectionView{}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{},
		},
		view: view,
	}

	p.OnEdit("c1")
	if !dialog.showCalled {
		t.Fatal("expected dialog to be shown")
	}

	saved := &models.SSHConnection{ID: "c1", Name: "New"}
	dialog.onSave(saved)

	if store.saveConn != saved {
		t.Fatal("expected connection to be saved")
	}
	if len(view.data) != 1 || view.data[0].Name != "New" {
		t.Fatalf("expected refresh after save, got %+v", view.data)
	}
}

func TestConnectionPresenter_OnEdit_SaveError(t *testing.T) {
	origFactory := newConnectionDialog
	origShowError := showError
	t.Cleanup(func() {
		newConnectionDialog = origFactory
		showError = origShowError
	})

	dialog := &mockConnectionDialog{}
	newConnectionDialog = func(_ fyne.App, _ fyne.Window, _ *models.SSHConnection) connectionDialog {
		return dialog
	}

	var shown error
	showError = func(err error, _ fyne.Window) { shown = err }

	p := &ConnectionPresenter{
		services: connectionServices{
			store:         &mockConnectionStore{saveErr: errors.New("save failed")},
			clientManager: &mockConnectionClientManager{},
		},
		view: &mockConnectionView{},
	}

	p.OnEdit("")
	dialog.onSave(&models.SSHConnection{ID: "c2"})

	if shown == nil || shown.Error() != "save failed" {
		t.Fatalf("expected save error dialog, got %v", shown)
	}
}

func TestConnectionPresenter_OnTestSuccess(t *testing.T) {
	origRun := runWithLoading
	origInfo := showInformation
	origErr := showError
	origDo := fyneDo
	origClient := newSSHTestClient
	origTest := testSSHConnection
	t.Cleanup(func() {
		runWithLoading = origRun
		showInformation = origInfo
		showError = origErr
		fyneDo = origDo
		newSSHTestClient = origClient
		testSSHConnection = origTest
	})
	fyneDo = func(fn func()) { fn() }

	var infoTitle, infoMessage string
	showInformation = func(title, message string, _ fyne.Window) {
		infoTitle = title
		infoMessage = message
	}
	showError = func(err error, _ fyne.Window) {
		t.Fatalf("unexpected error: %v", err)
	}
	runWithLoading = func(_ fyne.Window, _ string, _ string, _ time.Duration, task func(ctx context.Context) error, done func(ui.Result, error)) {
		done(ui.ResultSuccess, task(context.Background()))
	}
	newSSHTestClient = func(conn *models.SSHConnection) *ssh.Client { return ssh.NewClient(conn) }
	testSSHConnection = func(client *ssh.Client) error { return nil }

	store := &mockConnectionStore{
		byID: map[string]*models.SSHConnection{
			"c1": {ID: "c1", Name: "Alpha", Host: "host1", Port: 22},
		},
	}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{},
		},
	}

	p.OnTest("c1")

	if infoTitle == "" || infoMessage == "" {
		t.Fatal("expected success information dialog to be shown")
	}
}

func TestConnectionPresenter_OnTestError(t *testing.T) {
	origRun := runWithLoading
	origInfo := showInformation
	origErr := showError
	origDo := fyneDo
	origClient := newSSHTestClient
	origTest := testSSHConnection
	t.Cleanup(func() {
		runWithLoading = origRun
		showInformation = origInfo
		showError = origErr
		fyneDo = origDo
		newSSHTestClient = origClient
		testSSHConnection = origTest
	})
	fyneDo = func(fn func()) { fn() }

	var shown error
	showInformation = func(_, _ string, _ fyne.Window) {
		t.Fatal("did not expect success dialog")
	}
	showError = func(err error, _ fyne.Window) { shown = err }
	runWithLoading = func(_ fyne.Window, _ string, _ string, _ time.Duration, task func(ctx context.Context) error, done func(ui.Result, error)) {
		done(ui.ResultError, task(context.Background()))
	}
	newSSHTestClient = func(conn *models.SSHConnection) *ssh.Client { return ssh.NewClient(conn) }
	testSSHConnection = func(client *ssh.Client) error { return errors.New("dial failed") }

	store := &mockConnectionStore{
		byID: map[string]*models.SSHConnection{
			"c1": {ID: "c1", Name: "Alpha", Host: "host1", Port: 22},
		},
	}
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         store,
			clientManager: &mockConnectionClientManager{},
		},
	}

	p.OnTest("c1")

	if shown == nil || shown.Error() == "" {
		t.Fatal("expected error dialog on failed test")
	}
}

func TestConnectionPresenter_OnTestMissingConnection(t *testing.T) {
	p := &ConnectionPresenter{
		services: connectionServices{
			store:         &mockConnectionStore{byID: map[string]*models.SSHConnection{}},
			clientManager: &mockConnectionClientManager{},
		},
	}
	p.OnTest("missing")
}

func TestTunnelPresenter_RefreshData(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	store := &mockTunnelStore{
		tunnels: []*models.Tunnel{
			{ID: "t1", Name: "Web", LocalPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80},
		},
	}
	view := &mockTunnelView{}
	p := &TunnelPresenter{
		services: tunnelServices{
			store: store,
			manager: &mockTunnelManager{
				statuses:  map[string]models.TunnelStatus{"t1": models.StatusConnected},
				running:   map[string]bool{"t1": true},
				startErrs: map[string]error{},
			},
		},
		view: view,
	}

	p.RefreshData()

	if len(view.data) != 1 {
		t.Fatalf("expected 1 tunnel item, got %d", len(view.data))
	}
	if !view.data[0].IsRunning {
		t.Fatal("expected tunnel to be running")
	}
}

func TestNewTunnelPresenter_AndSetView(t *testing.T) {
	core, err := appcore.NewAppAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppAt: %v", err)
	}
	t.Cleanup(core.Shutdown)

	fyneApp := fynetest.NewApp()
	t.Cleanup(fyneApp.Quit)
	window := fyneApp.NewWindow("test")
	view := &mockTunnelView{}

	p := NewTunnelPresenter(core, fyneApp, window)
	p.SetView(view)

	if p.services.store == nil || p.services.manager == nil {
		t.Fatal("expected presenter services to be initialized")
	}
	if view.onEdit == nil || view.onDelete == nil || view.onStart == nil || view.onStop == nil || view.onStartAll == nil || view.onStopAll == nil {
		t.Fatal("expected tunnel callbacks to be wired")
	}
}

func TestTunnelPresenter_OnStartAllStartsOnlyStoppedTunnels(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{},
		running:   map[string]bool{"t1": false, "t2": true},
		startErrs: map[string]error{},
	}
	store := &mockTunnelStore{
		tunnels: []*models.Tunnel{
			{ID: "t1", Name: "One"},
			{ID: "t2", Name: "Two"},
		},
	}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: &mockTunnelView{},
	}

	p.OnStartAll()
	time.Sleep(20 * time.Millisecond)

	if len(manager.startedIDs) != 1 || manager.startedIDs[0] != "t1" {
		t.Fatalf("expected only stopped tunnel t1 to start, got %v", manager.startedIDs)
	}
}

func TestTunnelPresenter_OnEdit_SaveSuccess(t *testing.T) {
	origFactory := newTunnelDialog
	origDo := fyneDo
	t.Cleanup(func() {
		newTunnelDialog = origFactory
		fyneDo = origDo
	})
	fyneDo = func(fn func()) { fn() }

	dialog := &mockTunnelDialog{}
	newTunnelDialog = func(_ fyne.App, _ fyne.Window, tunnel *models.Tunnel, connections []*models.SSHConnection) tunnelDialog {
		if tunnel == nil || len(connections) != 1 {
			t.Fatal("expected existing tunnel and connections to be passed")
		}
		return dialog
	}

	store := &mockTunnelStore{
		tunnelsByID: map[string]*models.Tunnel{
			"t1": {ID: "t1", Name: "Old"},
		},
		connections: []*models.SSHConnection{{ID: "c1", Name: "Main"}},
	}
	view := &mockTunnelView{}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: &mockTunnelManager{statuses: map[string]models.TunnelStatus{}, running: map[string]bool{}, startErrs: map[string]error{}},
		},
		view: view,
	}

	p.OnEdit("t1")
	if !dialog.showCalled {
		t.Fatal("expected tunnel dialog to be shown")
	}

	saved := &models.Tunnel{ID: "t1", Name: "New", LocalPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80}
	dialog.onSave(saved)

	if store.savedTunnel != saved {
		t.Fatal("expected tunnel to be saved")
	}
	if len(view.data) != 1 || view.data[0].Name != "New" {
		t.Fatalf("expected refresh after save, got %+v", view.data)
	}
}

func TestTunnelPresenter_OnEdit_SaveError(t *testing.T) {
	origFactory := newTunnelDialog
	origShowError := showError
	t.Cleanup(func() {
		newTunnelDialog = origFactory
		showError = origShowError
	})

	dialog := &mockTunnelDialog{}
	newTunnelDialog = func(_ fyne.App, _ fyne.Window, _ *models.Tunnel, _ []*models.SSHConnection) tunnelDialog {
		return dialog
	}

	var shown error
	showError = func(err error, _ fyne.Window) { shown = err }

	p := &TunnelPresenter{
		services: tunnelServices{
			store:   &mockTunnelStore{saveErr: errors.New("save failed"), connections: []*models.SSHConnection{}},
			manager: &mockTunnelManager{statuses: map[string]models.TunnelStatus{}, running: map[string]bool{}, startErrs: map[string]error{}},
		},
		view: &mockTunnelView{},
	}

	p.OnEdit("")
	dialog.onSave(&models.Tunnel{ID: "t2"})

	if shown == nil || shown.Error() != "save failed" {
		t.Fatalf("expected save error dialog, got %v", shown)
	}
}

func TestTunnelPresenter_OnDeleteConfirmed(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{},
		running:   map[string]bool{},
		startErrs: map[string]error{},
	}
	store := &mockTunnelStore{}
	view := &mockTunnelView{confirmResult: true}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnDelete("t1")
	time.Sleep(20 * time.Millisecond)

	if len(manager.stoppedIDs) != 1 || manager.stoppedIDs[0] != "t1" {
		t.Fatalf("expected tunnel t1 to be stopped before delete, got %v", manager.stoppedIDs)
	}
	if store.deletedID != "t1" {
		t.Fatalf("expected deleted tunnel t1, got %q", store.deletedID)
	}
}

func TestTunnelPresenter_OnDeleteNotConfirmed(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{},
		running:   map[string]bool{},
		startErrs: map[string]error{},
	}
	store := &mockTunnelStore{}
	view := &mockTunnelView{confirmResult: false}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnDelete("t1")
	time.Sleep(20 * time.Millisecond)

	if len(manager.stoppedIDs) != 0 || store.deletedID != "" {
		t.Fatalf("expected no delete when not confirmed, stopped=%v deleted=%q", manager.stoppedIDs, store.deletedID)
	}
}

func TestTunnelPresenter_OnDeleteError(t *testing.T) {
	origDo := fyneDo
	origShowError := showError
	t.Cleanup(func() {
		fyneDo = origDo
		showError = origShowError
	})
	fyneDo = func(fn func()) { fn() }

	var shown error
	showError = func(err error, _ fyne.Window) { shown = err }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{},
		running:   map[string]bool{},
		startErrs: map[string]error{},
	}
	store := &mockTunnelStore{deleteErr: errors.New("delete failed")}
	view := &mockTunnelView{confirmResult: true}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnDelete("t1")
	time.Sleep(20 * time.Millisecond)

	if shown == nil || shown.Error() != "delete failed" {
		t.Fatalf("expected delete error dialog, got %v", shown)
	}
}

func TestTunnelPresenter_OnStartAllContinuesOnError(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{},
		running:   map[string]bool{"t1": false, "t2": false},
		startErrs: map[string]error{"t1": errors.New("boom")},
	}
	store := &mockTunnelStore{
		tunnels: []*models.Tunnel{
			{ID: "t1", Name: "One"},
			{ID: "t2", Name: "Two"},
		},
	}
	view := &mockTunnelView{}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnStartAll()
	time.Sleep(20 * time.Millisecond)

	if len(manager.startedIDs) != 2 {
		t.Fatalf("expected both tunnels to be attempted, got %v", manager.startedIDs)
	}
	if len(view.data) != 2 {
		t.Fatalf("expected refresh after start all, got %+v", view.data)
	}
}

func TestTunnelPresenter_OnStart_Error(t *testing.T) {
	origDo := fyneDo
	origShowError := showError
	t.Cleanup(func() {
		fyneDo = origDo
		showError = origShowError
	})
	fyneDo = func(fn func()) { fn() }

	var shown error
	showError = func(err error, _ fyne.Window) { shown = err }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{"t1": models.StatusDisconnected},
		running:   map[string]bool{"t1": false},
		startErrs: map[string]error{"t1": errors.New("start failed")},
	}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   &mockTunnelStore{tunnels: []*models.Tunnel{{ID: "t1", Name: "One"}}},
			manager: manager,
		},
		view: &mockTunnelView{},
	}

	p.OnStart("t1")
	time.Sleep(20 * time.Millisecond)

	if shown == nil || shown.Error() == "" {
		t.Fatal("expected start error dialog")
	}
}

func TestTunnelPresenter_OnStopAndStopAll(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{"t1": models.StatusDisconnected},
		running:   map[string]bool{"t1": false},
		startErrs: map[string]error{},
	}
	store := &mockTunnelStore{tunnels: []*models.Tunnel{{ID: "t1", Name: "One"}}}
	view := &mockTunnelView{}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnStop("t1")
	p.OnStopAll()
	time.Sleep(30 * time.Millisecond)

	if len(manager.stoppedIDs) == 0 || manager.stoppedIDs[0] != "t1" {
		t.Fatalf("expected tunnel stop to be recorded, got %v", manager.stoppedIDs)
	}
	if !manager.stopAllCalled {
		t.Fatal("expected StopAll to be called")
	}
}

func TestTunnelPresenter_OnStatusChangeRefreshes(t *testing.T) {
	origDo := fyneDo
	t.Cleanup(func() { fyneDo = origDo })
	fyneDo = func(fn func()) { fn() }

	store := &mockTunnelStore{
		tunnels: []*models.Tunnel{
			{ID: "t1", Name: "One", LocalPort: 9000, TargetHost: "127.0.0.1", TargetPort: 80},
		},
	}
	manager := &mockTunnelManager{
		statuses:  map[string]models.TunnelStatus{"t1": models.StatusError},
		running:   map[string]bool{"t1": false},
		startErrs: map[string]error{},
	}
	view := &mockTunnelView{}
	p := &TunnelPresenter{
		services: tunnelServices{
			store:   store,
			manager: manager,
		},
		view: view,
	}

	p.OnStatusChange("t1", models.StatusError, fmt.Errorf("boom"))

	if len(view.data) != 1 {
		t.Fatalf("expected refresh on status change, got %d items", len(view.data))
	}
	if view.data[0].Status == "" {
		t.Fatal("expected status text to be populated")
	}
}

func TestPresenter_DefaultWrappers(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	connDialog := newConnectionDialog(app, window, nil)
	connDialog.SetOnSave(func(*models.SSHConnection) {})
	connDialog.Show()

	tunnelDialog := newTunnelDialog(app, window, nil, []*models.SSHConnection{{ID: "conn-1", Name: "Main"}})
	tunnelDialog.SetOnSave(func(*models.Tunnel) {})
	tunnelDialog.Show()

	doneCh := make(chan ui.Result, 1)
	runWithLoading(window, "Title", "Loading", time.Second, func(context.Context) error {
		return nil
	}, func(result ui.Result, err error) {
		if err != nil {
			t.Fatalf("expected nil error from default loading wrapper, got %v", err)
		}
		doneCh <- result
	})

	select {
	case result := <-doneCh:
		if result != ui.ResultSuccess {
			t.Fatalf("expected success result, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for loading wrapper")
	}

	showError(errors.New("boom"), window)
	showInformation("Info", "done", window)
}

func init() {
	i18n.SetLanguage("en")
}
