package views

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

func TestConnectionView_SetDataAndCallbacks(t *testing.T) {
	i18n.SetLanguage("en")

	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewConnectionView(app, window)

	var testedID, editedID, deletedID string
	view.SetOnTest(func(id string) { testedID = id })
	view.SetOnEdit(func(id string) { editedID = id })
	view.SetOnDelete(func(id string) { deletedID = id })

	view.SetData([]ConnectionItem{
		{ID: "conn-1", Name: "Primary", Address: "127.0.0.1:22", IsConnected: true},
	})

	if got := view.statusLabel.Text; got != "1 connections, 1 connected" {
		t.Fatalf("unexpected status text: %q", got)
	}

	row := view.createRow()
	view.updateRow(0, row)

	leftBox, rightBox, addrLabel := splitRowContainers(row)
	if addrLabel.Text != "127.0.0.1:22" {
		t.Fatalf("unexpected address: %q", addrLabel.Text)
	}

	dot := leftBox.Objects[0].(*statusDot)
	gotDot := dot.fill.(color.RGBA)
	if gotDot.G != 175 {
		t.Fatalf("expected connected dot to be green, got %#v", gotDot)
	}

	testBtn := rightBox.Objects[0].(*widget.Button)
	editBtn := rightBox.Objects[1].(*widget.Button)
	deleteBtn := rightBox.Objects[2].(*widget.Button)

	testBtn.OnTapped()
	editBtn.OnTapped()
	deleteBtn.OnTapped()

	if testedID != "conn-1" || editedID != "conn-1" || deletedID != "conn-1" {
		t.Fatalf("unexpected callbacks: test=%q edit=%q delete=%q", testedID, editedID, deletedID)
	}

	editedID = "reset"
	view.newBtn.OnTapped()
	if editedID != "" {
		t.Fatalf("expected toolbar new button to call edit with empty id, got %q", editedID)
	}
}

func TestStatusDot_SetFillRebuildsResource(t *testing.T) {
	dot := newStatusDot()
	dot.SetFill(color.NRGBA{R: 1, G: 2, B: 3, A: 255})

	if dot.img.Resource == nil {
		t.Fatal("status dot image resource should be set")
	}
	if dot.img.Resource.Name() != "dot_1_2_3" {
		t.Fatalf("unexpected resource name: %s", dot.img.Resource.Name())
	}
}

func TestStatusDotRenderer_Methods(t *testing.T) {
	dot := newStatusDot()
	renderer := dot.CreateRenderer()

	renderer.Layout(fyne.NewSize(20, 20))
	if len(renderer.Objects()) != 1 {
		t.Fatalf("expected one renderer object, got %d", len(renderer.Objects()))
	}
	if renderer.MinSize().Width == 0 {
		t.Fatal("expected non-zero min size")
	}

	renderer.Refresh()
	renderer.Destroy()
	(&statusDotRenderer{img: dot.img}).Destroy()
}

func TestConnectionView_LanguageChangeAndHelpers(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewConnectionView(app, window)
	view.SetData([]ConnectionItem{
		{ID: "1", Name: "A", Address: "127.0.0.1:22", IsConnected: true},
		{ID: "2", Name: "B", Address: "127.0.0.1:23", IsConnected: false},
	})

	view.refreshStatusText()
	if got := view.statusLabel.Text; got != "2 connections, 1 connected" {
		t.Fatalf("unexpected status text: %q", got)
	}

	i18n.SetLanguage("en")
	i18n.NotifyLanguageChange()
	if view.newBtn.Text != "New" {
		t.Fatalf("expected localized button text, got %q", view.newBtn.Text)
	}
	if view.Container() == nil {
		t.Fatal("expected container")
	}
}

func TestConnectionView_ShowConfirm_UsesWrapper(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")
	view := NewConnectionView(app, window)

	origConfirm := showConnectionConfirmDialog
	t.Cleanup(func() { showConnectionConfirmDialog = origConfirm })

	called := false
	showConnectionConfirmDialog = func(title, message string, callback func(bool), parent fyne.Window) {
		if title != "Confirm" || message != "Question" || parent != window {
			t.Fatalf("unexpected confirm args: %q %q %v", title, message, parent)
		}
		called = true
		callback(true)
	}

	confirmed := false
	view.ShowConfirm("Confirm", "Question", func(ok bool) { confirmed = ok })

	if !called || !confirmed {
		t.Fatal("expected confirm wrapper to be invoked")
	}
}

func TestConnectionView_UpdateRow_DisconnectedAndInvalidObject(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewConnectionView(app, window)
	view.SetData([]ConnectionItem{
		{ID: "conn-2", Name: "Secondary", Address: "127.0.0.1:23", IsConnected: false},
	})

	view.updateRow(0, widget.NewLabel("wrong"))

	row := view.createRow()
	view.updateRow(0, row)

	leftBox, _, _ := splitRowContainers(row)
	dot := leftBox.Objects[0].(*statusDot)
	gotDot := dot.fill.(color.RGBA)
	if gotDot.R != 158 || gotDot.G != 158 || gotDot.B != 158 {
		t.Fatalf("expected disconnected dot to be gray, got %#v", gotDot)
	}
}

func TestConnectionView_UpdateRow_InvalidButtons(t *testing.T) {
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("test")

	view := NewConnectionView(app, window)
	view.SetData([]ConnectionItem{
		{ID: "conn-3", Name: "Broken", Address: "127.0.0.1:24", IsConnected: true},
	})

	row := view.createRow()
	_, rightBox, _ := splitRowContainers(row)
	rightBox.Objects[0] = widget.NewLabel("bad")

	view.updateRow(0, row)
}
