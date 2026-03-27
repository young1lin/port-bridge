package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/updater"
	"github.com/young1lin/port-bridge/internal/version"
)

func makeUpdateNowAction(win fyne.Window, release *updater.ReleaseInfo, u updateApplier, hide func()) func() {
	return func() {
		hide()
		showUpdateProgressDialog(win, release, u)
	}
}

// ShowUpdateAvailableDialog displays a dialog informing the user a new version is available.
func ShowUpdateAvailableDialog(win fyne.Window, release *updater.ReleaseInfo, u updateApplier) {
	currentLabel := widget.NewLabel(fmt.Sprintf(i18n.L("A new version %s is available!"), release.TagName))
	currentLabel.TextStyle = fyne.TextStyle{Bold: true}
	versionLabel := widget.NewLabel(fmt.Sprintf(i18n.L("Current version: %s"), version.ShortVersion()))
	versionLabel.Importance = widget.LowImportance

	releaseNotesLabel := widget.NewLabel(i18n.L("Release Notes:"))
	releaseNotesLabel.TextStyle = fyne.TextStyle{Bold: true}

	releaseNotes := widget.NewLabel(release.Body)
	releaseNotes.Wrapping = fyne.TextWrapWord
	releaseNotes.Importance = widget.LowImportance

	scroll := container.NewScroll(releaseNotes)
	scroll.SetMinSize(fyne.NewSize(400, 150))

	content := container.NewVBox(
		currentLabel,
		versionLabel,
		widget.NewSeparator(),
		releaseNotesLabel,
		scroll,
	)

	dlg := dialog.NewCustom(i18n.L("Update Available"), "", content, win)

	dlg.SetButtons([]fyne.CanvasObject{
		widget.NewButton(i18n.L("Later"), dlg.Hide),
		widget.NewButton(i18n.L("Update Now"), makeUpdateNowAction(win, release, u, dlg.Hide)),
	})
	dlg.Show()
}
