package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
	"github.com/young1lin/port-bridge/internal/updater"
)

// ShowUpdateProgressDialog displays a progress dialog during update download and installation.
func ShowUpdateProgressDialog(win fyne.Window, release *updater.ReleaseInfo, u updateApplier) {
	statusLabel := widget.NewLabel(i18n.L("Downloading: 0.0 / 0.0 MB"))
	statusLabel.Alignment = fyne.TextAlignCenter
	progressBar := widget.NewProgressBar()
	progressBar.Min = 0
	progressBar.Max = 1

	content := container.NewVBox(
		progressBar,
		statusLabel,
	)

	dlg := dialog.NewCustom(i18n.L("Update Available"), "", content, win)
	dlg.SetButtons(nil) // no close button during update
	dlg.Show()

	go func() {
		progressCb := func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			fraction := float64(downloaded) / float64(total)
			fyne.Do(func() {
				statusLabel.SetText(fmt.Sprintf(i18n.L("Downloading: %.1f / %.1f MB"),
					float64(downloaded)/(1024*1024),
					float64(total)/(1024*1024)))
				progressBar.SetValue(fraction)
			})
		}

		fyne.Do(func() {
			statusLabel.SetText(i18n.L("Downloading..."))
		})

		err := u.DownloadAndApply(release, progressCb)

		if err != nil {
			fyne.Do(func() {
				dlg.Hide()
				showErrorDialog(fmt.Errorf(i18n.L("Update failed: %v"), err), win)
			})
			return
		}

		// If we reach here, the app should have been restarted
		fyne.Do(func() {
			statusLabel.SetText(i18n.L("Update successful! Restarting..."))
			progressBar.SetValue(1)
		})
	}()
}
