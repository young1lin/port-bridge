package dialogs

import (
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"github.com/young1lin/port-bridge/internal/updater"
)

type updateApplier interface {
	DownloadAndApply(release *updater.ReleaseInfo, progress updater.ProgressCallback) error
}

var showErrorDialog = dialog.ShowError
var showInformationDialog = dialog.ShowInformation
var showUpdateProgressDialog = ShowUpdateProgressDialog
var userHomeDir = os.UserHomeDir
var newFileURI = storage.NewFileURI
var listerForURI = storage.ListerForURI

type fileOpenDialog interface {
	SetLocation(location fyne.ListableURI)
	Show()
}

var newFileOpenDialog = func(callback func(fyne.URIReadCloser, error), parent fyne.Window) fileOpenDialog {
	return dialog.NewFileOpen(callback, parent)
}

// newHPadded wraps content with horizontal-only padding (no extra vertical padding).
// Use this instead of double NewPadded when you only need left/right breathing room.
func newHPadded(obj fyne.CanvasObject) *fyne.Container {
	const hPad = float32(8)
	l := canvas.NewRectangle(color.Transparent)
	l.SetMinSize(fyne.NewSize(hPad, 0))
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(hPad, 0))
	return container.NewBorder(nil, nil, l, r, obj)
}
