package dialogs

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ui/windowguard"
)

// ConnectionDialog is the dialog for editing SSH connections
type ConnectionDialog struct {
	app          fyne.App
	parentWindow fyne.Window
	dialogWindow fyne.Window
	conn         *models.SSHConnection
	onSave       func(*models.SSHConnection)

	// Form fields
	nameEntry          *widget.Entry
	hostEntry          *widget.Entry
	portEntry          *widget.Entry
	usernameEntry      *widget.Entry
	authTypeSelect     *widget.Select
	passwordEntry      *widget.Entry
	keyPathEntry       *widget.Entry
	keyPassphraseEntry *widget.Entry

	// Pre-created auth section widgets
	passwordSection *fyne.Container
	keySection      *fyne.Container

	// Proxy fields
	useProxyCheck  *widget.Check
	proxyHostEntry *widget.Entry
	proxyPortEntry *widget.Entry
	proxyUserEntry *widget.Entry
	proxyPassEntry *widget.Entry
	proxySection   *fyne.Container
}

// NewConnectionDialog creates a new connection dialog
func NewConnectionDialog(app fyne.App, window fyne.Window, conn *models.SSHConnection) *ConnectionDialog {
	cd := &ConnectionDialog{
		app:          app,
		parentWindow: window,
	}

	if conn != nil {
		cd.conn = conn
	} else {
		cd.conn = models.NewSSHConnection()
	}

	cd.setupUI()
	return cd
}

// setupUI sets up the dialog UI
func (cd *ConnectionDialog) setupUI() {
	log.Println("[DEBUG] ConnectionDialog.setupUI started")

	// === Basic fields ===
	cd.nameEntry = widget.NewEntry()
	cd.nameEntry.SetPlaceHolder(i18n.L("Enter connection name"))
	cd.nameEntry.SetText(cd.conn.Name)

	cd.hostEntry = widget.NewEntry()
	cd.hostEntry.SetPlaceHolder(i18n.L("e.g. 192.168.1.100"))
	cd.hostEntry.SetText(cd.conn.Host)

	cd.portEntry = widget.NewEntry()
	cd.portEntry.SetPlaceHolder("22")
	if cd.conn.Port > 0 {
		cd.portEntry.SetText(fmt.Sprintf("%d", cd.conn.Port))
	} else {
		cd.portEntry.SetText("22")
	}

	cd.usernameEntry = widget.NewEntry()
	cd.usernameEntry.SetPlaceHolder(i18n.L("Enter username"))
	cd.usernameEntry.SetText(cd.conn.Username)

	// === Password auth fields ===
	cd.passwordEntry = widget.NewPasswordEntry()
	cd.passwordEntry.SetPlaceHolder(i18n.L("Enter password"))
	cd.passwordEntry.SetText(cd.conn.Password)

	cd.passwordSection = container.NewVBox(
		widget.NewLabel(i18n.L("Password:")),
		cd.passwordEntry,
	)

	// === Key auth fields ===
	cd.keyPathEntry = widget.NewEntry()
	cd.keyPathEntry.SetPlaceHolder("~/.ssh/id_rsa")
	cd.keyPathEntry.SetText(cd.conn.KeyPath)

	cd.keyPassphraseEntry = widget.NewPasswordEntry()
	cd.keyPassphraseEntry.SetPlaceHolder(i18n.L("Key passphrase, leave empty if none"))
	cd.keyPassphraseEntry.SetText(cd.conn.KeyPassphrase)

	browseBtn := widget.NewButton(i18n.L("Browse..."), cd.showFileDialog)
	keyRow := container.NewBorder(nil, nil, nil, browseBtn, cd.keyPathEntry)

	cd.keySection = container.NewVBox(
		widget.NewLabel(i18n.L("Key Path:")),
		keyRow,
		widget.NewLabel(i18n.L("Key Passphrase (optional):")),
		cd.keyPassphraseEntry,
	)

	// === Auth type select ===
	authType := string(cd.conn.AuthType)
	if authType == "" {
		authType = "password"
	}

	cd.authTypeSelect = widget.NewSelect([]string{"password", "key"}, cd.onAuthTypeChanged)
	cd.authTypeSelect.SetSelected(authType)

	// === Proxy fields ===
	cd.useProxyCheck = widget.NewCheck(i18n.L("Use SOCKS5 proxy"), cd.onProxyCheckChanged)
	cd.useProxyCheck.Checked = cd.conn.UseProxy

	cd.proxyHostEntry = widget.NewEntry()
	cd.proxyHostEntry.SetPlaceHolder(i18n.L("Proxy server address"))
	cd.proxyHostEntry.SetText(cd.conn.ProxyHost)

	cd.proxyPortEntry = widget.NewEntry()
	cd.proxyPortEntry.SetPlaceHolder("1080")
	if cd.conn.ProxyPort > 0 {
		cd.proxyPortEntry.SetText(fmt.Sprintf("%d", cd.conn.ProxyPort))
	} else {
		cd.proxyPortEntry.SetText("1080")
	}

	cd.proxyUserEntry = widget.NewEntry()
	cd.proxyUserEntry.SetPlaceHolder(i18n.L("Username (optional)"))
	cd.proxyUserEntry.SetText(cd.conn.ProxyUsername)

	cd.proxyPassEntry = widget.NewPasswordEntry()
	cd.proxyPassEntry.SetPlaceHolder(i18n.L("Password (optional)"))
	cd.proxyPassEntry.SetText(cd.conn.ProxyPassword)

	// Pre-create proxy section (will show/hide based on checkbox)
	proxyForm := widget.NewForm(
		&widget.FormItem{Text: i18n.L("Proxy Host"), Widget: cd.proxyHostEntry},
		&widget.FormItem{Text: i18n.L("Proxy Port"), Widget: cd.proxyPortEntry},
		&widget.FormItem{Text: i18n.L("Proxy User"), Widget: cd.proxyUserEntry},
		&widget.FormItem{Text: i18n.L("Proxy Password"), Widget: cd.proxyPassEntry},
	)
	cd.proxySection = container.NewVBox(proxyForm)

	// === Build form ===
	form := widget.NewForm(
		&widget.FormItem{Text: i18n.L("Name"), Widget: cd.nameEntry},
		&widget.FormItem{Text: i18n.L("Host"), Widget: cd.hostEntry},
		&widget.FormItem{Text: i18n.L("Port"), Widget: cd.portEntry},
		&widget.FormItem{Text: i18n.L("Username"), Widget: cd.usernameEntry},
		&widget.FormItem{Text: i18n.L("Auth Method"), Widget: cd.authTypeSelect},
	)

	// === Auth container (holds either password or key section) ===
	authContainer := container.NewStack(cd.passwordSection, cd.keySection)

	// === Buttons ===
	saveBtn := widget.NewButton(i18n.L("Save"), cd.handleSubmit)
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(i18n.L("Cancel"), func() {
		cd.dialogWindow.Close()
	})

	buttonBox := container.NewCenter(container.NewHBox(cancelBtn, saveBtn))

	// === Main content ===
	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		widget.NewLabel(i18n.L("Authentication")),
		authContainer,
		widget.NewSeparator(),
		widget.NewLabel(i18n.L("Proxy Settings")),
		cd.useProxyCheck,
		cd.proxySection,
	)

	// Initial visibility setup
	cd.onAuthTypeChanged(cd.authTypeSelect.Selected)
	cd.onProxyCheckChanged(cd.useProxyCheck.Checked)

	// === Window setup ===
	title := i18n.L("New Connection")
	if cd.conn.Name != "" {
		title = i18n.L("Edit Connection")
	}

	cd.dialogWindow = cd.app.NewWindow(title)
	unlockParent := windowguard.ProtectParentWindow(cd.parentWindow)
	cd.dialogWindow.SetOnClosed(unlockParent)
	cd.dialogWindow.SetContent(container.NewBorder(
		nil,
		container.NewPadded(buttonBox),
		nil,
		nil,
		container.NewScroll(newHPadded(content)),
	))
	cd.dialogWindow.Resize(fyne.NewSize(480, 520))
	cd.dialogWindow.SetFixedSize(true)
	cd.dialogWindow.CenterOnScreen()

	log.Println("[DEBUG] ConnectionDialog.setupUI completed")
}

// onAuthTypeChanged handles auth type selection change
func (cd *ConnectionDialog) onAuthTypeChanged(authType string) {
	if authType == "key" {
		cd.passwordSection.Hide()
		cd.keySection.Show()
	} else {
		cd.keySection.Hide()
		cd.passwordSection.Show()
	}
}

// onProxyCheckChanged handles proxy checkbox change
func (cd *ConnectionDialog) onProxyCheckChanged(checked bool) {
	if checked {
		cd.proxySection.Show()
	} else {
		cd.proxySection.Hide()
	}
}

// showFileDialog shows the file browser dialog
func (cd *ConnectionDialog) showFileDialog() {
	homeDir, err := userHomeDir()
	if err != nil {
		homeDir = "."
	}
	sshDir := filepath.Join(homeDir, ".ssh")

	fileDialog := newFileOpenDialog(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		cd.keyPathEntry.SetText(reader.URI().Path())
		reader.Close()
	}, cd.dialogWindow)

	sshURI := newFileURI(sshDir)
	if listableURI, err := listerForURI(sshURI); err == nil {
		fileDialog.SetLocation(listableURI)
	}

	fileDialog.Show()
}

// handleSubmit handles form submission
func (cd *ConnectionDialog) handleSubmit() {
	// Validate
	if cd.nameEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter connection name")), cd.dialogWindow)
		return
	}
	if cd.hostEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter host address")), cd.dialogWindow)
		return
	}
	if cd.usernameEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter username")), cd.dialogWindow)
		return
	}

	// Save values
	cd.conn.Name = cd.nameEntry.Text
	cd.conn.Host = cd.hostEntry.Text
	cd.conn.Username = cd.usernameEntry.Text
	cd.conn.AuthType = models.AuthType(cd.authTypeSelect.Selected)

	if port, err := strconv.Atoi(cd.portEntry.Text); err == nil && port >= 1 && port <= 65535 {
		cd.conn.Port = port
	} else {
		cd.conn.Port = 22
	}

	// Save auth data
	if cd.conn.AuthType == models.AuthTypePassword {
		cd.conn.Password = cd.passwordEntry.Text
	} else {
		cd.conn.KeyPath = cd.keyPathEntry.Text
		cd.conn.KeyPassphrase = cd.keyPassphraseEntry.Text
	}

	// Save proxy config
	cd.conn.UseProxy = cd.useProxyCheck.Checked
	if cd.conn.UseProxy {
		cd.conn.ProxyHost = cd.proxyHostEntry.Text
		if port, err := strconv.Atoi(cd.proxyPortEntry.Text); err == nil {
			cd.conn.ProxyPort = port
		} else {
			cd.conn.ProxyPort = 1080
		}
		cd.conn.ProxyUsername = cd.proxyUserEntry.Text
		cd.conn.ProxyPassword = cd.proxyPassEntry.Text
	}

	if cd.onSave != nil {
		cd.onSave(cd.conn)
	}
	cd.dialogWindow.Close()
}

// Show shows the dialog
func (cd *ConnectionDialog) Show() {
	cd.dialogWindow.Show()
}

// SetOnSave sets the save callback
func (cd *ConnectionDialog) SetOnSave(callback func(*models.SSHConnection)) {
	cd.onSave = callback
}
