package dialogs

import (
	"errors"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ui/windowguard"
)

// TunnelDialog is the dialog for editing tunnel rules
type TunnelDialog struct {
	app          fyne.App
	parentWindow fyne.Window
	dialogWindow fyne.Window
	tunnel       *models.Tunnel
	connections  []*models.SSHConnection
	onSave       func(*models.Tunnel)

	// Form fields
	nameEntry              *widget.Entry
	localPortEntry         *widget.Entry
	connectionSelect       *widget.Select
	targetHostEntry        *widget.Entry
	targetPortEntry        *widget.Entry
	remarkEntry            *widget.Entry
	autoReconnectCheck     *widget.Check
	reconnectIntervalEntry *widget.Entry
	allowLANCheck          *widget.Check
}

// NewTunnelDialog creates a new tunnel dialog
func NewTunnelDialog(app fyne.App, window fyne.Window, tunnel *models.Tunnel, connections []*models.SSHConnection) *TunnelDialog {
	td := &TunnelDialog{
		app:          app,
		parentWindow: window,
		connections:  connections,
	}

	if tunnel != nil {
		td.tunnel = tunnel
	} else {
		td.tunnel = models.NewTunnel()
	}

	td.setupUI()
	return td
}

// setupUI sets up the dialog UI
func (td *TunnelDialog) setupUI() {
	// Name field
	td.nameEntry = widget.NewEntry()
	td.nameEntry.SetPlaceHolder(i18n.L("Enter rule name"))
	td.nameEntry.SetText(td.tunnel.Name)

	// Local port field
	td.localPortEntry = widget.NewEntry()
	td.localPortEntry.SetPlaceHolder(i18n.L("Local listening port"))
	if td.tunnel.LocalPort > 0 {
		td.localPortEntry.SetText(fmt.Sprintf("%d", td.tunnel.LocalPort))
	}

	// Connection select
	connOptions := make([]string, len(td.connections))
	for i, conn := range td.connections {
		connOptions[i] = conn.Name
	}
	td.connectionSelect = widget.NewSelect(connOptions, func(s string) {})
	if td.tunnel.ConnectionID != "" {
		for _, conn := range td.connections {
			if conn.ID == td.tunnel.ConnectionID {
				td.connectionSelect.SetSelected(conn.Name)
				break
			}
		}
	}

	// Target host field
	td.targetHostEntry = widget.NewEntry()
	td.targetHostEntry.SetPlaceHolder(i18n.L("Target host address"))
	td.targetHostEntry.SetText(td.tunnel.TargetHost)

	// Target port field
	td.targetPortEntry = widget.NewEntry()
	td.targetPortEntry.SetPlaceHolder(i18n.L("Target port"))
	if td.tunnel.TargetPort > 0 {
		td.targetPortEntry.SetText(fmt.Sprintf("%d", td.tunnel.TargetPort))
	}

	// Remark field
	td.remarkEntry = widget.NewEntry()
	td.remarkEntry.SetPlaceHolder(i18n.L("Remark (optional)"))
	td.remarkEntry.SetText(td.tunnel.Remark)

	// Auto reconnect checkbox
	td.autoReconnectCheck = widget.NewCheck(i18n.L("Enable auto reconnect"), func(b bool) {
		td.tunnel.AutoReconnect = b
	})
	td.autoReconnectCheck.Checked = td.tunnel.AutoReconnect

	// Reconnect interval field
	td.reconnectIntervalEntry = widget.NewEntry()
	td.reconnectIntervalEntry.SetPlaceHolder("10")
	if td.tunnel.ReconnectInterval > 0 {
		td.reconnectIntervalEntry.SetText(fmt.Sprintf("%d", td.tunnel.ReconnectInterval))
	} else {
		td.reconnectIntervalEntry.SetText("10")
	}

	// Allow LAN checkbox with security warning
	td.allowLANCheck = widget.NewCheck(i18n.L("Allow LAN access (0.0.0.0)"), func(b bool) {
		if b {
			// Show security warning when user enables AllowLAN
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   i18n.L("Security Warning"),
				Content: i18n.L("Allowing LAN access will bind to all network interfaces (0.0.0.0). This may expose the tunnel to other devices on your network. Ensure you trust your network environment."),
			})
		}
		td.tunnel.AllowLAN = b
	})
	td.allowLANCheck.Checked = td.tunnel.AllowLAN

	// Build form
	formItems := []*widget.FormItem{
		{Text: i18n.L("Name"), Widget: td.nameEntry},
		{Text: i18n.L("SSH Connection"), Widget: td.connectionSelect},
		{Text: i18n.L("Local Port"), Widget: td.localPortEntry},
		{Text: i18n.L("Target Host"), Widget: td.targetHostEntry},
		{Text: i18n.L("Target Port"), Widget: td.targetPortEntry},
		{Text: i18n.L("Remark"), Widget: td.remarkEntry},
	}

	form := widget.NewForm(formItems...)

	// Advanced settings section
	advancedSection := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel(i18n.L("Advanced Settings")),
		td.allowLANCheck,
		td.autoReconnectCheck,
		container.NewHBox(
			widget.NewLabel(i18n.L("Reconnect interval:")),
			td.reconnectIntervalEntry,
			widget.NewLabel(i18n.L("seconds")),
		),
	)

	content := container.NewVBox(
		form,
		advancedSection,
	)

	// Buttons
	saveBtn := widget.NewButton(i18n.L("Save"), func() {
		td.handleSubmit()
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(i18n.L("Cancel"), func() {
		td.dialogWindow.Close()
	})

	buttonBox := container.NewHBox(
		container.NewCenter(container.NewHBox(cancelBtn, saveBtn)),
	)

	// Dialog title
	title := i18n.L("New Forwarding Rule")
	if td.tunnel.Name != "" {
		title = i18n.L("Edit Forwarding Rule")
	}

	// Create a new independent window
	td.dialogWindow = td.app.NewWindow(title)
	unlockParent := windowguard.ProtectParentWindow(td.parentWindow)
	td.dialogWindow.SetOnClosed(unlockParent)
	td.dialogWindow.SetContent(container.NewBorder(
		nil,
		container.NewPadded(buttonBox),
		nil,
		nil,
		container.NewScroll(newHPadded(content)),
	))
	td.dialogWindow.Resize(fyne.NewSize(420, 420))
	td.dialogWindow.SetFixedSize(true)
	td.dialogWindow.CenterOnScreen()

	// Set close intercept
	td.dialogWindow.SetCloseIntercept(func() {
		td.dialogWindow.Close()
	})
}

// handleSubmit handles form submission
func (td *TunnelDialog) handleSubmit() {
	// Validate required fields
	if td.nameEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter rule name")), td.dialogWindow)
		return
	}
	if td.connectionSelect.Selected == "" && len(td.connections) > 0 {
		showErrorDialog(errors.New(i18n.L("Please select an SSH connection")), td.dialogWindow)
		return
	}
	if td.localPortEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter local port")), td.dialogWindow)
		return
	}
	if td.targetHostEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter target host")), td.dialogWindow)
		return
	}
	if td.targetPortEntry.Text == "" {
		showErrorDialog(errors.New(i18n.L("Please enter target port")), td.dialogWindow)
		return
	}

	td.tunnel.Name = td.nameEntry.Text

	// Find connection ID from name
	for _, conn := range td.connections {
		if conn.Name == td.connectionSelect.Selected {
			td.tunnel.ConnectionID = conn.ID
			break
		}
	}

	localPort, localErr := strconv.Atoi(td.localPortEntry.Text)
	if localErr != nil || localPort < 1 || localPort > 65535 {
		showErrorDialog(errors.New(i18n.L("Local port must be between 1 and 65535")), td.dialogWindow)
		return
	}
	td.tunnel.LocalPort = localPort

	td.tunnel.TargetHost = td.targetHostEntry.Text

	targetPort, targetErr := strconv.Atoi(td.targetPortEntry.Text)
	if targetErr != nil || targetPort < 1 || targetPort > 65535 {
		showErrorDialog(errors.New(i18n.L("Target port must be between 1 and 65535")), td.dialogWindow)
		return
	}
	td.tunnel.TargetPort = targetPort

	td.tunnel.Remark = td.remarkEntry.Text
	td.tunnel.AutoReconnect = td.autoReconnectCheck.Checked
	td.tunnel.AllowLAN = td.allowLANCheck.Checked

	if interval, err := strconv.Atoi(td.reconnectIntervalEntry.Text); err == nil {
		td.tunnel.ReconnectInterval = interval
	} else {
		td.tunnel.ReconnectInterval = 10
	}

	if td.onSave != nil {
		td.onSave(td.tunnel)
	}
	td.dialogWindow.Close()
}

// Show shows the dialog
func (td *TunnelDialog) Show() {
	td.dialogWindow.Show()
}

// SetOnSave sets the save callback
func (td *TunnelDialog) SetOnSave(callback func(*models.Tunnel)) {
	td.onSave = callback
}
