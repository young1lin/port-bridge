package views

import (
	"errors"
	"fmt"
	"image/color"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

var (
	showErrorDialog   = dialog.ShowError
	showConfirmDialog = dialog.ShowConfirm
	showInfoDialog    = dialog.ShowInformation
)

// fixedWidthLayout constrains widgets to a fixed width column
type fixedWidthLayout struct {
	width float32
}

func (l *fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objects {
		obj.Move(fyne.NewPos(0, 0))
		obj.Resize(fyne.NewSize(l.width, size.Height))
	}
}

func (l *fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	minH := float32(0)
	for _, obj := range objects {
		if h := obj.MinSize().Height; h > minH {
			minH = h
		}
	}
	return fyne.NewSize(l.width, minH)
}

// TunnelView displays the list of port forwarding tunnels
type TunnelView struct {
	app         fyne.App
	window      fyne.Window
	container   *fyne.Container
	list        *widget.List
	statusLabel *widget.Label
	data        []TunnelItem
	newBtn      *widget.Button
	startAllBtn *widget.Button
	stopAllBtn  *widget.Button
	onEdit      func(id string)
	onDelete    func(id string)
	onStart     func(id string)
	onStop      func(id string)
	onStartAll  func()
	onStopAll   func()
}

// TunnelItem represents a single tunnel in the list
type TunnelItem struct {
	ID          string
	Name        string
	LocalPort   int
	Target      string
	Status      string
	StatusColor color.Color
	IsRunning   bool
}

// rowWidgets holds widget references for a row
type rowWidgets struct {
	statusIcon *statusDot
	name       *widget.Label
	addr       *widget.Label
	status     *widget.Label
	startBtn   *widget.Button
	stopBtn    *widget.Button
	editBtn    *widget.Button
	deleteBtn  *widget.Button
}

// NewTunnelView creates a new tunnel view
func NewTunnelView(app fyne.App, window fyne.Window) *TunnelView {
	tv := &TunnelView{
		app:    app,
		window: window,
		data:   make([]TunnelItem, 0),
	}

	tv.setupUI()
	tv.registerLanguageChange()
	return tv
}

// setupUI sets up the UI components
func (tv *TunnelView) setupUI() {
	tv.list = widget.NewList(
		func() int { return len(tv.data) },
		func() fyne.CanvasObject {
			return tv.createRow()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			tv.updateRow(id, obj)
		},
	)

	// Toolbar buttons with icons
	tv.newBtn = widget.NewButtonWithIcon(i18n.L("New"), theme.ContentAddIcon(), func() {
		if tv.onEdit != nil {
			tv.onEdit("")
		}
	})
	tv.newBtn.Importance = widget.HighImportance

	tv.startAllBtn = widget.NewButtonWithIcon(i18n.L("Start All"), theme.MediaPlayIcon(), func() {
		if tv.onStartAll != nil {
			tv.onStartAll()
		}
	})

	tv.stopAllBtn = widget.NewButtonWithIcon(i18n.L("Stop All"), theme.MediaStopIcon(), func() {
		if tv.onStopAll != nil {
			tv.onStopAll()
		}
	})
	tv.stopAllBtn.Importance = widget.DangerImportance

	// Top bar: toolbar + separator
	topBar := container.NewVBox(
		container.NewHBox(tv.newBtn, tv.startAllBtn, tv.stopAllBtn),
		widget.NewSeparator(),
	)

	// Bottom bar: separator + right-aligned status
	tv.statusLabel = widget.NewLabel("")
	bottomBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			tv.statusLabel,
		),
	)

	tv.container = container.NewBorder(topBar, bottomBar, nil, nil, tv.list)
}

// registerLanguageChange registers a callback to refresh UI on language change.
func (tv *TunnelView) registerLanguageChange() {
	i18n.OnLanguageChange(func() {
		tv.newBtn.SetText(i18n.L("New"))
		tv.startAllBtn.SetText(i18n.L("Start All"))
		tv.stopAllBtn.SetText(i18n.L("Stop All"))
		tv.list.Refresh()
		tv.refreshStatusText()
	})
}

// refreshStatusText updates the status label using current language and data.
func (tv *TunnelView) refreshStatusText() {
	activeCount := 0
	for _, item := range tv.data {
		if item.IsRunning {
			activeCount++
		}
	}
	tv.statusLabel.SetText(fmt.Sprintf(i18n.L("%d rules, %d active"), len(tv.data), activeCount))
}

// createRow creates a row template
func (tv *TunnelView) createRow() *fyne.Container {
	// Status dot (SVG circle)
	statusDot := newStatusDot()

	// Name (fixed width 150px, truncate on overflow)
	nameLabel := widget.NewLabel(i18n.L("Rule Name"))
	nameLabel.Truncation = fyne.TextTruncateEllipsis
	nameWrapper := container.New(&fixedWidthLayout{width: 150}, nameLabel)

	// Connection info
	addrLabel := widget.NewLabel("0 -> 0.0.0.0:0")

	// Toggle start/stop button
	toggleBtn := widget.NewButtonWithIcon(i18n.L("Start"), theme.MediaPlayIcon(), nil)
	toggleBtn.Importance = widget.HighImportance

	// Edit button
	editBtn := widget.NewButtonWithIcon(i18n.L("Edit"), theme.DocumentCreateIcon(), nil)

	// Delete button
	deleteBtn := widget.NewButtonWithIcon(i18n.L("Delete"), theme.DeleteIcon(), nil)
	deleteBtn.Importance = widget.DangerImportance

	// Left: status dot (centered) + name (fixed width)
	leftBox := container.NewHBox(statusDot, nameWrapper)

	// Right buttons: toggle + edit + delete
	rightBox := container.NewHBox(
		toggleBtn,
		editBtn,
		deleteBtn,
	)

	// Border layout: left fixed, right fixed, center address expands
	// Top/bottom transparent padding to prevent bottom clipping in widget.List
	topSpacer := canvas.NewRectangle(color.Transparent)
	topSpacer.SetMinSize(fyne.NewSize(0, rowPad))
	botSpacer := canvas.NewRectangle(color.Transparent)
	botSpacer.SetMinSize(fyne.NewSize(0, rowPad))

	return container.NewBorder(topSpacer, botSpacer, leftBox, rightBox, addrLabel)
}

// updateRow updates a row with data
func (tv *TunnelView) updateRow(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(tv.data) {
		return
	}

	item := tv.data[id]
	border, ok := obj.(*fyne.Container)
	if !ok {
		log.Printf("[ERROR] obj is not *fyne.Container: %T", obj)
		return
	}

	// Border layout: Objects include top/bottom spacers (skipped) plus left/right/center
	var leftBox, rightBox *fyne.Container
	var addrLabel *widget.Label

	for _, o := range border.Objects {
		if c, ok := o.(*fyne.Container); ok && len(c.Objects) > 0 {
			switch c.Objects[0].(type) {
			case *fyne.Container, *statusDot:
				leftBox = c
			case *widget.Button:
				rightBox = c
			}
		} else if label, ok := o.(*widget.Label); ok {
			addrLabel = label
		}
	}

	// Update left: status dot + name
	if leftBox != nil && len(leftBox.Objects) >= 2 {
		// Status dot color
		if dot, ok := leftBox.Objects[0].(*statusDot); ok {
			dot.SetFill(item.StatusColor)
		}
		// Name (in fixed width container)
		if wrapper, ok := leftBox.Objects[1].(*fyne.Container); ok && len(wrapper.Objects) > 0 {
			if label, ok := wrapper.Objects[0].(*widget.Label); ok {
				label.SetText(item.Name)
			}
		}
	}

	// Update center: connection info (local port -> target address)
	if addrLabel != nil {
		addrLabel.SetText(fmt.Sprintf("%d -> %s", item.LocalPort, item.Target))
	}

	// Update right buttons (3: toggle, edit, delete)
	if rightBox != nil && len(rightBox.Objects) >= 3 {
		toggleBtn, ok1 := rightBox.Objects[0].(*widget.Button)
		editBtn, ok2 := rightBox.Objects[1].(*widget.Button)
		deleteBtn, ok3 := rightBox.Objects[2].(*widget.Button)
		if !ok1 || !ok2 || !ok3 {
			log.Printf("[ERROR] tunnel row button type assertion failed")
			return
		}

		// Update button labels for current language
		editBtn.SetText(i18n.L("Edit"))
		editBtn.SetIcon(theme.DocumentCreateIcon())
		deleteBtn.SetText(i18n.L("Delete"))
		deleteBtn.SetIcon(theme.DeleteIcon())

		if !item.IsRunning {
			toggleBtn.SetText(i18n.L("Start"))
			toggleBtn.SetIcon(theme.MediaPlayIcon())
			toggleBtn.Importance = widget.HighImportance
			toggleBtn.Enable()
			toggleBtn.OnTapped = func() {
				if tv.onStart != nil {
					tv.onStart(item.ID)
				}
			}
		} else {
			toggleBtn.SetText(i18n.L("Stop"))
			toggleBtn.SetIcon(theme.MediaStopIcon())
			toggleBtn.Importance = widget.DangerImportance
			toggleBtn.Enable()
			toggleBtn.OnTapped = func() {
				if tv.onStop != nil {
					tv.onStop(item.ID)
				}
			}
		}

		if item.IsRunning {
			editBtn.Disable()
			editBtn.OnTapped = func() {
				showInfoDialog(i18n.L("Edit"), i18n.L("Please stop the tunnel before editing."), tv.window)
			}
			deleteBtn.Disable()
			deleteBtn.OnTapped = func() {
				showInfoDialog(i18n.L("Delete"), i18n.L("Please stop the tunnel before deleting."), tv.window)
			}
		} else {
			editBtn.Enable()
			editBtn.OnTapped = func() {
				if tv.onEdit != nil {
					tv.onEdit(item.ID)
				}
			}
			deleteBtn.Enable()
			deleteBtn.OnTapped = func() {
				if tv.onDelete != nil {
					tv.onDelete(item.ID)
				}
			}
		}
	}
}

// Container returns the view container
func (tv *TunnelView) Container() fyne.CanvasObject {
	return tv.container
}

// SetData sets the tunnel data
func (tv *TunnelView) SetData(data []TunnelItem) {
	log.Printf("[DEBUG] TunnelView.SetData: count=%d", len(data))
	tv.data = data
	tv.list.Refresh()

	activeCount := 0
	for _, item := range data {
		if item.IsRunning {
			activeCount++
		}
	}
	tv.statusLabel.SetText(fmt.Sprintf(i18n.L("%d rules, %d active"), len(tv.data), activeCount))
}

// SetOnEdit sets the edit callback
func (tv *TunnelView) SetOnEdit(callback func(id string)) {
	tv.onEdit = callback
}

// SetOnDelete sets the delete callback
func (tv *TunnelView) SetOnDelete(callback func(id string)) {
	tv.onDelete = callback
}

// SetOnStart sets the start callback
func (tv *TunnelView) SetOnStart(callback func(id string)) {
	tv.onStart = callback
}

// SetOnStop sets the stop callback
func (tv *TunnelView) SetOnStop(callback func(id string)) {
	tv.onStop = callback
}

// SetOnStartAll sets the start all callback
func (tv *TunnelView) SetOnStartAll(callback func()) {
	tv.onStartAll = callback
}

// SetOnStopAll sets the stop all callback
func (tv *TunnelView) SetOnStopAll(callback func()) {
	tv.onStopAll = callback
}

// ShowError shows an error dialog
func (tv *TunnelView) ShowError(title, message string) {
	showErrorDialog(errors.New(message), tv.window)
}

// ShowConfirm shows a confirmation dialog
func (tv *TunnelView) ShowConfirm(title, message string, callback func(bool)) {
	showConfirmDialog(title, message, callback, tv.window)
}
