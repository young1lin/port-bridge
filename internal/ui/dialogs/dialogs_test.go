package dialogs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	fynetest "fyne.io/fyne/v2/test"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/updater"
)

type fakeUpdater struct {
	checkRelease   *updater.ReleaseInfo
	checkErr       error
	downloadErr    error
	progressCalled atomic.Bool
}

func (f *fakeUpdater) CheckForUpdate() (*updater.ReleaseInfo, error) {
	return f.checkRelease, f.checkErr
}

func (f *fakeUpdater) DownloadAndApply(release *updater.ReleaseInfo, progress updater.ProgressCallback) error {
	progress(512, 1024)
	f.progressCalled.Store(true)
	return f.downloadErr
}

type fakeFileOpenDialog struct {
	callback      func(fyne.URIReadCloser, error)
	location      fyne.ListableURI
	showCalled    bool
	selectedPath  string
	callbackError error
}

func (d *fakeFileOpenDialog) SetLocation(location fyne.ListableURI) {
	d.location = location
}

func (d *fakeFileOpenDialog) Show() {
	d.showCalled = true
	if d.selectedPath != "" || d.callbackError != nil {
		d.callback(&fakeURIReadCloser{uri: storage.NewFileURI(d.selectedPath)}, d.callbackError)
	}
}

type fakeURIReadCloser struct {
	uri fyne.URI
}

func (r *fakeURIReadCloser) URI() fyne.URI            { return r.uri }
func (r *fakeURIReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (r *fakeURIReadCloser) Close() error             { return nil }

func TestConnectionDialog_HandleSubmitPassword(t *testing.T) {
	i18n.SetLanguage("en")
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)

	dialog.nameEntry.SetText("prod")
	dialog.hostEntry.SetText("127.0.0.1")
	dialog.portEntry.SetText("2222")
	dialog.usernameEntry.SetText("root")
	dialog.passwordEntry.SetText("secret")

	var saved *models.SSHConnection
	dialog.SetOnSave(func(conn *models.SSHConnection) { saved = conn.Clone() })
	dialog.handleSubmit()

	if saved == nil {
		t.Fatal("expected save callback")
	}
	if saved.Port != 2222 || saved.Password != "secret" || saved.AuthType != models.AuthTypePassword {
		t.Fatalf("unexpected saved connection: %+v", saved)
	}
}

func TestConnectionDialog_HandleSubmitValidation(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origShowError := showErrorDialog
	t.Cleanup(func() { showErrorDialog = origShowError })

	var got error
	showErrorDialog = func(err error, _ fyne.Window) {
		got = err
	}

	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)
	dialog.handleSubmit()

	if got == nil || got.Error() != "Please enter connection name" {
		t.Fatalf("unexpected validation error: %v", got)
	}
}

func TestConnectionDialog_HandleSubmitValidationBranches(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	origShowError := showErrorDialog
	t.Cleanup(func() { showErrorDialog = origShowError })

	var got error
	showErrorDialog = func(err error, _ fyne.Window) { got = err }

	dialog := NewConnectionDialog(app, window, nil)
	dialog.nameEntry.SetText("name")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please enter host address" {
		t.Fatalf("expected host validation, got %v", got)
	}

	got = nil
	dialog.hostEntry.SetText("127.0.0.1")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please enter username" {
		t.Fatalf("expected username validation, got %v", got)
	}
}

func TestConnectionDialog_HandleSubmitKeyAndProxyDefaults(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)

	dialog.nameEntry.SetText("proxy")
	dialog.hostEntry.SetText("ssh.example.com")
	dialog.usernameEntry.SetText("alice")
	dialog.authTypeSelect.SetSelected("key")
	dialog.keyPathEntry.SetText("C:/id_rsa")
	dialog.keyPassphraseEntry.SetText("phrase")
	dialog.useProxyCheck.SetChecked(true)
	dialog.proxyHostEntry.SetText("127.0.0.1")
	dialog.proxyPortEntry.SetText("bad")
	dialog.proxyUserEntry.SetText("bob")
	dialog.proxyPassEntry.SetText("pw")

	var saved *models.SSHConnection
	dialog.SetOnSave(func(conn *models.SSHConnection) { saved = conn.Clone() })
	dialog.handleSubmit()

	if saved == nil {
		t.Fatal("expected save callback")
	}
	if saved.AuthType != models.AuthTypeKey || saved.KeyPath != "C:/id_rsa" {
		t.Fatalf("unexpected key auth save: %+v", saved)
	}
	if !saved.UseProxy || saved.ProxyPort != 1080 {
		t.Fatalf("expected proxy default port 1080, got %+v", saved)
	}
}

func TestConnectionDialog_Show(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	dialog := NewConnectionDialog(app, window, nil)
	if !dialog.dialogWindow.FixedSize() {
		t.Fatal("connection dialog window should be fixed size")
	}
	dialog.Show()
}

func TestConnectionDialog_ShowFileDialog(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)

	origHome := userHomeDir
	origNewFileOpen := newFileOpenDialog
	t.Cleanup(func() {
		userHomeDir = origHome
		newFileOpenDialog = origNewFileOpen
	})

	fd := &fakeFileOpenDialog{selectedPath: "/tmp/id_rsa"}
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}
	userHomeDir = func() (string, error) { return home, nil }
	newFileOpenDialog = func(callback func(fyne.URIReadCloser, error), parent fyne.Window) fileOpenDialog {
		fd.callback = callback
		return fd
	}

	dialog.showFileDialog()

	if !fd.showCalled {
		t.Fatal("expected file dialog to be shown")
	}
	if fd.location == nil {
		t.Fatal("expected file dialog location to be set")
	}
	if dialog.keyPathEntry.Text != "/tmp/id_rsa" {
		t.Fatalf("expected selected key path to be applied, got %q", dialog.keyPathEntry.Text)
	}
}

func TestConnectionDialog_ShowFileDialog_Fallbacks(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)

	origHome := userHomeDir
	origNewFileOpen := newFileOpenDialog
	t.Cleanup(func() {
		userHomeDir = origHome
		newFileOpenDialog = origNewFileOpen
	})

	fd := &fakeFileOpenDialog{callbackError: errors.New("picker failed")}
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	newFileOpenDialog = func(callback func(fyne.URIReadCloser, error), parent fyne.Window) fileOpenDialog {
		fd.callback = callback
		return fd
	}

	dialog.showFileDialog()

	if !fd.showCalled {
		t.Fatal("expected file dialog to show even when home lookup fails")
	}
	if dialog.keyPathEntry.Text != "" {
		t.Fatalf("expected key path to stay empty on dialog error, got %q", dialog.keyPathEntry.Text)
	}
}

func TestConnectionDialog_ExistingConnectionInitializesFields(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	conn := &models.SSHConnection{
		Name:          "prod",
		Host:          "10.0.0.1",
		Port:          2200,
		Username:      "alice",
		AuthType:      models.AuthTypeKey,
		KeyPath:       "/tmp/id_rsa",
		KeyPassphrase: "secret",
		UseProxy:      true,
		ProxyHost:     "127.0.0.1",
		ProxyPort:     1081,
		ProxyUsername: "bob",
		ProxyPassword: "pw",
	}

	dialog := NewConnectionDialog(app, window, conn)

	if dialog.nameEntry.Text != "prod" || dialog.hostEntry.Text != "10.0.0.1" || dialog.portEntry.Text != "2200" {
		t.Fatalf("unexpected connection fields: %q %q %q", dialog.nameEntry.Text, dialog.hostEntry.Text, dialog.portEntry.Text)
	}
	if dialog.authTypeSelect.Selected != "key" || dialog.keyPathEntry.Text != "/tmp/id_rsa" {
		t.Fatalf("expected key auth fields to be restored, got auth=%q key=%q", dialog.authTypeSelect.Selected, dialog.keyPathEntry.Text)
	}
	if !dialog.proxySection.Visible() || !dialog.useProxyCheck.Checked {
		t.Fatal("expected proxy section to be visible for proxied connection")
	}
}

func TestConnectionDialog_DefaultStateAndToggles(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	dialog := NewConnectionDialog(app, window, nil)

	if dialog.authTypeSelect.Selected != "password" {
		t.Fatalf("expected default auth type password, got %q", dialog.authTypeSelect.Selected)
	}
	if !dialog.passwordSection.Visible() || dialog.keySection.Visible() {
		t.Fatal("expected password section visible and key section hidden by default")
	}
	if dialog.proxySection.Visible() {
		t.Fatal("expected proxy section hidden by default")
	}

	dialog.onAuthTypeChanged("key")
	dialog.onProxyCheckChanged(true)
	if dialog.passwordSection.Visible() || !dialog.keySection.Visible() {
		t.Fatal("expected key auth toggle to switch visible section")
	}
	if !dialog.proxySection.Visible() {
		t.Fatal("expected proxy section visible after enabling proxy")
	}
}

func TestConnectionDialog_HandleSubmit_DefaultPortAndNoProxy(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")
	dialog := NewConnectionDialog(app, window, nil)

	dialog.nameEntry.SetText("prod")
	dialog.hostEntry.SetText("127.0.0.1")
	dialog.portEntry.SetText("bad")
	dialog.usernameEntry.SetText("root")
	dialog.passwordEntry.SetText("secret")
	dialog.useProxyCheck.SetChecked(false)

	var saved *models.SSHConnection
	dialog.SetOnSave(func(conn *models.SSHConnection) { saved = conn.Clone() })
	dialog.handleSubmit()

	if saved == nil {
		t.Fatal("expected save callback")
	}
	if saved.Port != 22 {
		t.Fatalf("expected invalid port to fall back to 22, got %d", saved.Port)
	}
	if saved.UseProxy || saved.ProxyHost != "" || saved.ProxyPort != 0 {
		t.Fatalf("expected proxy fields to remain empty when proxy disabled, got %+v", saved)
	}
}

func TestTunnelDialog_HandleSubmitAndDefaults(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	connections := []*models.SSHConnection{{ID: "conn-1", Name: "Main"}}
	dialog := NewTunnelDialog(app, window, nil, connections)
	dialog.nameEntry.SetText("web")
	dialog.connectionSelect.SetSelected("Main")
	dialog.localPortEntry.SetText("8080")
	dialog.targetHostEntry.SetText("10.0.0.1")
	dialog.targetPortEntry.SetText("80")
	dialog.reconnectIntervalEntry.SetText("bad")
	dialog.allowLANCheck.SetChecked(true)
	dialog.autoReconnectCheck.SetChecked(true)

	var saved *models.Tunnel
	dialog.SetOnSave(func(tunnel *models.Tunnel) { saved = tunnel.Clone() })
	dialog.handleSubmit()

	if saved == nil {
		t.Fatal("expected save callback")
	}
	if saved.ConnectionID != "conn-1" || saved.ReconnectInterval != 10 || !saved.AllowLAN || !saved.AutoReconnect {
		t.Fatalf("unexpected saved tunnel: %+v", saved)
	}
}

func TestTunnelDialog_ExistingTunnelInitializesFields(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	connections := []*models.SSHConnection{{ID: "conn-1", Name: "Main"}}
	tunnel := &models.Tunnel{
		Name:              "web",
		ConnectionID:      "conn-1",
		LocalPort:         8080,
		TargetHost:        "127.0.0.1",
		TargetPort:        80,
		Remark:            "demo",
		AutoReconnect:     true,
		ReconnectInterval: 15,
		AllowLAN:          true,
	}

	dialog := NewTunnelDialog(app, window, tunnel, connections)

	if dialog.nameEntry.Text != "web" || dialog.connectionSelect.Selected != "Main" {
		t.Fatalf("unexpected tunnel field initialization: %q %q", dialog.nameEntry.Text, dialog.connectionSelect.Selected)
	}
	if dialog.localPortEntry.Text != "8080" || dialog.targetPortEntry.Text != "80" {
		t.Fatalf("unexpected port initialization: local=%q target=%q", dialog.localPortEntry.Text, dialog.targetPortEntry.Text)
	}
	if !dialog.autoReconnectCheck.Checked || !dialog.allowLANCheck.Checked || dialog.reconnectIntervalEntry.Text != "15" {
		t.Fatalf("unexpected advanced settings initialization")
	}
}

func TestTunnelDialog_DefaultStateAndClose(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	dialog := NewTunnelDialog(app, window, nil, nil)
	if dialog.connectionSelect.Selected != "" || len(dialog.connectionSelect.Options) != 0 {
		t.Fatalf("expected empty connection options, got selected=%q options=%d", dialog.connectionSelect.Selected, len(dialog.connectionSelect.Options))
	}
	if dialog.reconnectIntervalEntry.Text != "10" {
		t.Fatalf("expected default reconnect interval 10, got %q", dialog.reconnectIntervalEntry.Text)
	}

	dialog.dialogWindow.Close()
}

func TestTunnelDialog_HandleSubmit_InvalidPortRejected(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	connections := []*models.SSHConnection{{ID: "conn-1", Name: "Main"}}
	tunnel := &models.Tunnel{LocalPort: 7000, TargetPort: 9000}
	dialog := NewTunnelDialog(app, window, tunnel, connections)
	dialog.nameEntry.SetText("rule")
	dialog.connectionSelect.SetSelected("Main")
	dialog.targetHostEntry.SetText("127.0.0.1")

	var saved *models.Tunnel
	dialog.SetOnSave(func(t *models.Tunnel) { saved = t.Clone() })

	// Non-numeric local port → should be rejected
	dialog.localPortEntry.SetText("bad")
	dialog.targetPortEntry.SetText("80")
	dialog.handleSubmit()
	if saved != nil {
		t.Fatal("expected no save for invalid local port")
	}

	// Out-of-range local port → should be rejected
	dialog.localPortEntry.SetText("99999")
	dialog.handleSubmit()
	if saved != nil {
		t.Fatal("expected no save for out-of-range local port")
	}

	// Valid local port, invalid target port → should be rejected
	dialog.localPortEntry.SetText("8080")
	dialog.targetPortEntry.SetText("bad")
	dialog.handleSubmit()
	if saved != nil {
		t.Fatal("expected no save for invalid target port")
	}

	// Both valid → should save
	dialog.targetPortEntry.SetText("80")
	dialog.handleSubmit()
	if saved == nil {
		t.Fatal("expected save callback for valid ports")
	}
	if saved.LocalPort != 8080 || saved.TargetPort != 80 {
		t.Fatalf("unexpected saved ports: local=%d target=%d", saved.LocalPort, saved.TargetPort)
	}
}

func TestTunnelDialog_HandleSubmitValidation(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origShowError := showErrorDialog
	t.Cleanup(func() { showErrorDialog = origShowError })

	var got error
	showErrorDialog = func(err error, _ fyne.Window) {
		got = err
	}

	window := app.NewWindow("test")
	dialog := NewTunnelDialog(app, window, nil, []*models.SSHConnection{{ID: "conn-1", Name: "Main"}})
	dialog.handleSubmit()

	if got == nil || got.Error() != "Please enter rule name" {
		t.Fatalf("unexpected validation error: %v", got)
	}
}

func TestTunnelDialog_HandleSubmitValidationBranches(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origShowError := showErrorDialog
	t.Cleanup(func() { showErrorDialog = origShowError })

	var got error
	showErrorDialog = func(err error, _ fyne.Window) { got = err }

	window := app.NewWindow("test")
	dialog := NewTunnelDialog(app, window, nil, []*models.SSHConnection{{ID: "conn-1", Name: "Main"}})
	dialog.nameEntry.SetText("rule")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please select an SSH connection" {
		t.Fatalf("expected connection validation, got %v", got)
	}

	got = nil
	dialog.connectionSelect.SetSelected("Main")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please enter local port" {
		t.Fatalf("expected local port validation, got %v", got)
	}

	got = nil
	dialog.localPortEntry.SetText("8080")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please enter target host" {
		t.Fatalf("expected target host validation, got %v", got)
	}

	got = nil
	dialog.targetHostEntry.SetText("127.0.0.1")
	dialog.handleSubmit()
	if got == nil || got.Error() != "Please enter target port" {
		t.Fatalf("expected target port validation, got %v", got)
	}
}

func TestTunnelDialog_Show(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	dialog := NewTunnelDialog(app, window, nil, nil)
	if !dialog.dialogWindow.FixedSize() {
		t.Fatal("tunnel dialog window should be fixed size")
	}
	dialog.Show()
}

func TestShowUpdateProgressDialog_ReportsError(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)

	origShowError := showErrorDialog
	t.Cleanup(func() { showErrorDialog = origShowError })

	errCh := make(chan error, 1)
	showErrorDialog = func(err error, _ fyne.Window) {
		errCh <- err
	}

	window := app.NewWindow("test")
	u := &fakeUpdater{downloadErr: errors.New("network failed")}
	ShowUpdateProgressDialog(window, &updater.ReleaseInfo{TagName: "v1.0.0"}, u)

	select {
	case err := <-errCh:
		if err == nil || err.Error() == "" {
			t.Fatal("expected an error from update progress dialog")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for update error callback")
	}

	if !u.progressCalled.Load() {
		t.Fatal("expected progress callback to be invoked")
	}
}

func TestShowUpdateProgressDialog_Success(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	u := &fakeUpdater{}
	ShowUpdateProgressDialog(window, &updater.ReleaseInfo{TagName: "v1.0.0"}, u)

	time.Sleep(100 * time.Millisecond)
	if !u.progressCalled.Load() {
		t.Fatal("expected progress callback to be invoked")
	}
}

func TestMakeUpdateNowAction_InvokesProgressDialog(t *testing.T) {
	origShowProgress := showUpdateProgressDialog
	t.Cleanup(func() { showUpdateProgressDialog = origShowProgress })

	window := fynetest.NewApp().NewWindow("test")
	called := false
	hidden := false
	showUpdateProgressDialog = func(_ fyne.Window, _ *updater.ReleaseInfo, _ updateApplier) {
		called = true
	}

	action := makeUpdateNowAction(window, &updater.ReleaseInfo{TagName: "v1.0.0", Body: "notes"}, &fakeUpdater{}, func() {
		hidden = true
	})
	action()

	if !hidden {
		t.Fatal("expected dialog hide callback")
	}
	if !called {
		t.Fatal("expected update progress dialog callback")
	}
}

func TestShowUpdateAvailableDialog_Shows(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	ShowUpdateAvailableDialog(window, &updater.ReleaseInfo{TagName: "v1.0.0", Body: "notes"}, &fakeUpdater{})
}
