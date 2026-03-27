package views

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

func TestTunnelView_SetDataAndCallbacks(t *testing.T) {
	i18n.SetLanguage("en")

	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewTunnelView(app, window)

	var startedID, stoppedID, editedID, deletedID string
	view.SetOnStart(func(id string) { startedID = id })
	view.SetOnStop(func(id string) { stoppedID = id })
	view.SetOnEdit(func(id string) { editedID = id })
	view.SetOnDelete(func(id string) { deletedID = id })
	startAllCalled := false
	stopAllCalled := false
	view.SetOnStartAll(func() { startAllCalled = true })
	view.SetOnStopAll(func() { stopAllCalled = true })

	view.SetData([]TunnelItem{
		{ID: "rule-1", Name: "Rule", LocalPort: 8080, Target: "10.0.0.1:80", StatusColor: color.NRGBA{R: 76, G: 175, B: 80, A: 255}},
	})

	if got := view.statusLabel.Text; got != "1 rules, 0 active" {
		t.Fatalf("unexpected status text: %q", got)
	}

	row := view.createRow()
	view.updateRow(0, row)

	_, rightBox, addrLabel := splitRowContainers(row)
	if got := addrLabel.Text; got != "8080 -> 10.0.0.1:80" {
		t.Fatalf("unexpected tunnel address: %q", got)
	}

	toggleBtn := rightBox.Objects[0].(*widget.Button)
	editBtn := rightBox.Objects[1].(*widget.Button)
	deleteBtn := rightBox.Objects[2].(*widget.Button)

	toggleBtn.OnTapped()
	editBtn.OnTapped()
	deleteBtn.OnTapped()

	if startedID != "rule-1" || editedID != "rule-1" || deletedID != "rule-1" {
		t.Fatalf("unexpected callbacks: start=%q edit=%q delete=%q", startedID, editedID, deletedID)
	}
	if stoppedID != "" {
		t.Fatalf("stop callback should not fire, got %q", stoppedID)
	}

	editedID = "reset"
	view.newBtn.OnTapped()
	view.startAllBtn.OnTapped()
	view.stopAllBtn.OnTapped()
	if editedID != "" || !startAllCalled || !stopAllCalled {
		t.Fatalf("expected top-level buttons to invoke callbacks, got edit=%q startAll=%v stopAll=%v", editedID, startAllCalled, stopAllCalled)
	}
}

func TestTunnelView_RunningRowDisablesEditAndDelete(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewTunnelView(app, window)
	view.SetData([]TunnelItem{
		{ID: "rule-2", Name: "Rule", LocalPort: 9090, Target: "10.0.0.2:443", IsRunning: true, StatusColor: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
	})

	row := view.createRow()
	view.updateRow(0, row)

	_, rightBox, _ := splitRowContainers(row)
	toggleBtn := rightBox.Objects[0].(*widget.Button)
	editBtn := rightBox.Objects[1].(*widget.Button)
	deleteBtn := rightBox.Objects[2].(*widget.Button)

	if toggleBtn.Text != "Stop" {
		t.Fatalf("expected running tunnel button text Stop, got %q", toggleBtn.Text)
	}
	if !editBtn.Disabled() || !deleteBtn.Disabled() {
		t.Fatal("edit and delete should be disabled while tunnel is running")
	}
}

func TestTunnelView_UpdateRow_StopAndInvalidObjects(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewTunnelView(app, window)
	view.SetData([]TunnelItem{
		{ID: "rule-2", Name: "Rule", LocalPort: 9090, Target: "10.0.0.2:443", IsRunning: true, StatusColor: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
	})

	var stoppedID string
	view.SetOnStop(func(id string) { stoppedID = id })

	view.updateRow(0, widget.NewLabel("bad"))

	brokenRow := view.createRow()
	_, brokenButtons, _ := splitRowContainers(brokenRow)
	brokenButtons.Objects[0] = widget.NewLabel("bad")
	view.updateRow(0, brokenRow)

	row := view.createRow()
	view.updateRow(0, row)
	_, rightBox, _ := splitRowContainers(row)
	rightBox.Objects[0].(*widget.Button).OnTapped()

	if stoppedID != "rule-2" {
		t.Fatalf("expected stop callback for running tunnel, got %q", stoppedID)
	}

	view.updateRow(1, row)
}

func TestFixedWidthLayout(t *testing.T) {
	layout := &fixedWidthLayout{width: 150}
	label := widget.NewLabel("label")
	layout.Layout([]fyne.CanvasObject{label}, fyne.NewSize(300, 40))

	if got := label.Size().Width; got != 150 {
		t.Fatalf("expected width 150, got %v", got)
	}

	min := layout.MinSize([]fyne.CanvasObject{label})
	if min.Width != 150 {
		t.Fatalf("expected min width 150, got %v", min.Width)
	}
}

func TestTunnelView_HelpersAndDialogs(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewTunnelView(app, window)
	view.SetData([]TunnelItem{
		{ID: "1", Name: "One", LocalPort: 8080, Target: "1.1.1.1:80", IsRunning: true, Status: "Connected", StatusColor: color.NRGBA{R: 76, G: 175, B: 80, A: 255}},
		{ID: "2", Name: "Two", LocalPort: 9090, Target: "2.2.2.2:90", IsRunning: false, Status: "Disconnected", StatusColor: color.NRGBA{R: 158, G: 158, B: 158, A: 255}},
	})

	view.refreshStatusText()
	if got := view.statusLabel.Text; got != "2 rules, 1 active" {
		t.Fatalf("unexpected status text: %q", got)
	}
	if view.Container() == nil {
		t.Fatal("expected container")
	}

	view.SetOnStartAll(func() {})
	view.SetOnStopAll(func() {})

	origErr := showErrorDialog
	origConfirm := showConfirmDialog
	origInfo := showInfoDialog
	t.Cleanup(func() {
		showErrorDialog = origErr
		showConfirmDialog = origConfirm
		showInfoDialog = origInfo
	})

	errCalled := false
	showErrorDialog = func(err error, parent fyne.Window) {
		errCalled = true
		if err == nil || parent != window {
			t.Fatalf("unexpected error args: %v %v", err, parent)
		}
	}

	confirmCalled := false
	showConfirmDialog = func(title, message string, callback func(bool), parent fyne.Window) {
		confirmCalled = true
		callback(true)
		if parent != window {
			t.Fatalf("unexpected parent window: %v", parent)
		}
	}
	infoCalls := 0
	showInfoDialog = func(title, message string, parent fyne.Window) {
		infoCalls++
		if parent != window || title == "" || message == "" {
			t.Fatalf("unexpected info dialog args: %q %q %v", title, message, parent)
		}
	}

	view.ShowError("Error", "boom")
	confirmed := false
	view.ShowConfirm("Confirm", "Question", func(ok bool) { confirmed = ok })

	if !errCalled || !confirmCalled || !confirmed {
		t.Fatal("expected dialog wrappers to be invoked")
	}

	row := view.createRow()
	view.updateRow(0, row)
	_, rightBox, _ := splitRowContainers(row)
	rightBox.Objects[1].(*widget.Button).OnTapped()
	rightBox.Objects[2].(*widget.Button).OnTapped()
	if infoCalls != 2 {
		t.Fatalf("expected info dialogs for running tunnel actions, got %d", infoCalls)
	}

	i18n.SetLanguage("en")
	i18n.NotifyLanguageChange()
	if view.newBtn.Text != "New" || view.startAllBtn.Text != "Start All" || view.stopAllBtn.Text != "Stop All" {
		t.Fatalf("unexpected button texts after language refresh: %q %q %q", view.newBtn.Text, view.startAllBtn.Text, view.stopAllBtn.Text)
	}
}

func splitRowContainers(row *fyne.Container) (*fyne.Container, *fyne.Container, *widget.Label) {
	var leftBox, rightBox *fyne.Container
	var addrLabel *widget.Label

	for _, obj := range row.Objects {
		if c, ok := obj.(*fyne.Container); ok && len(c.Objects) > 0 {
			switch c.Objects[0].(type) {
			case *fyne.Container, *statusDot:
				leftBox = c
			case *widget.Button:
				rightBox = c
			}
		} else if label, ok := obj.(*widget.Label); ok {
			addrLabel = label
		}
	}

	return leftBox, rightBox, addrLabel
}
