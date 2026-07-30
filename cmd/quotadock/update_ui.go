package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	updater "github.com/jungdosa/QuotaDock/internal/update"
)

const updateCheckTimeout = 12 * time.Second

type updateController struct {
	rootContext   context.Context
	window        fyne.Window
	catalog       *i18n.Catalog
	language      func() (i18n.Language, i18n.Language)
	checker       updater.Checker
	flow          updater.Flow
	quit          func()
	preparePrompt func()
	busy          atomic.Bool
}

func (u *updateController) Check(manual bool) {
	if u == nil || !u.busy.CompareAndSwap(false, true) {
		return
	}
	var checking *widget.PopUp
	if manual {
		label := widget.NewLabel(u.text(i18n.KeyUpdateChecking))
		checking = u.newModalPopup(u.text(i18n.KeyUpdate), container.NewPadded(label), fyne.NewSize(360, 100))
		checking.Show()
	}
	go func() {
		checkContext, cancel := context.WithTimeout(u.rootContext, updateCheckTimeout)
		defer cancel()
		result := u.checker.Check(checkContext)
		fyne.Do(func() {
			if checking != nil {
				checking.Hide()
			}
			u.handleCheckResult(result, manual)
		})
	}()
}

func (u *updateController) handleCheckResult(result updater.CheckResult, manual bool) {
	switch result.Status {
	case updater.CheckAvailable:
		u.showAvailable(result.Release)
	case updater.CheckUpToDate:
		u.busy.Store(false)
		if manual {
			u.showInformation(u.text(i18n.KeyUpdateUpToDate))
		}
	default:
		u.busy.Store(false)
		if manual {
			u.showInformation(u.text(i18n.KeyUpdateCheckFailed))
		}
	}
}

func (u *updateController) showAvailable(release updater.Release) {
	if u.preparePrompt != nil {
		u.preparePrompt()
	}
	message := fmt.Sprintf(u.text(i18n.KeyUpdateAvailable), updater.DisplayVersion(release.TagName))
	if u.flow.Portable {
		message += "\n\n" + u.text(i18n.KeyUpdatePortableNotice)
	} else if u.flow.CanAutoInstall(release) {
		message += "\n\n" + u.text(i18n.KeyUpdateUnsignedNotice)
	}

	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	var prompt *widget.PopUp
	later := widget.NewButton(u.text(i18n.KeyUpdateLater), func() {
		prompt.Hide()
		u.busy.Store(false)
	})
	now := widget.NewButton(u.text(i18n.KeyUpdateNow), func() {
		prompt.Hide()
		u.apply(release)
	})
	now.Importance = widget.HighImportance
	buttons := container.NewHBox(later, now)
	content := container.NewVBox(container.NewGridWrap(fyne.NewSize(420, 96), label), container.NewCenter(buttons))
	prompt = u.newModalPopup(u.text(i18n.KeyUpdate), content, fyne.NewSize(460, 180))
	prompt.Show()
}

func (u *updateController) apply(release updater.Release) {
	if !u.flow.CanAutoInstall(release) {
		_, err := u.flow.Apply(u.rootContext, release, nil)
		if err != nil {
			slog.Warn("release page could not be opened", "error", err)
		}
		u.busy.Store(false)
		return
	}

	installContext, cancel := context.WithCancel(u.rootContext)
	status := widget.NewLabel(fmt.Sprintf(u.text(i18n.KeyUpdateDownloading), 0))
	progressBar := widget.NewProgressBar()
	var progressDialog *widget.PopUp
	cancelButton := widget.NewButton(u.text(i18n.KeyUpdateLater), func() {
		cancel()
		progressDialog.Hide()
	})
	content := container.NewVBox(
		container.NewGridWrap(fyne.NewSize(420, 32), status),
		progressBar,
		container.NewCenter(cancelButton),
	)
	progressDialog = u.newModalPopup(u.text(i18n.KeyUpdate), content, fyne.NewSize(460, 180))
	progressDialog.Show()

	go func() {
		_, err := u.flow.Apply(installContext, release, func(progress updater.Progress) {
			renderProgress := func() {
				switch progress.Stage {
				case updater.StageVerifying:
					status.SetText(u.text(i18n.KeyUpdateVerifying))
				case updater.StageInstalling:
					status.SetText(u.text(i18n.KeyUpdateInstalling))
					cancelButton.Disable()
				default:
					status.SetText(fmt.Sprintf(u.text(i18n.KeyUpdateDownloading), progress.Percent))
					progressBar.SetValue(float64(progress.Percent) / 100)
				}
			}
			if progress.Stage == updater.StageInstalling {
				fyne.DoAndWait(renderProgress)
			} else {
				fyne.Do(renderProgress)
			}
		})
		fyne.Do(func() {
			progressDialog.Hide()
			if err == nil {
				if u.quit != nil {
					u.quit()
				}
				return
			}
			u.busy.Store(false)
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, updater.ErrHashMismatch) {
				u.showInformation(u.text(i18n.KeyUpdateHashMismatch))
				return
			}
			slog.Warn("automatic update stopped", "error", err)
		})
	}()
}

func (u *updateController) showInformation(message string) {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	var popup *widget.PopUp
	done := widget.NewButton(u.text(i18n.KeyDone), func() { popup.Hide() })
	content := container.NewVBox(container.NewGridWrap(fyne.NewSize(360, 72), label), container.NewCenter(done))
	popup = u.newModalPopup(u.text(i18n.KeyUpdate), content, fyne.NewSize(400, 150))
	popup.Show()
}

func (u *updateController) newModalPopup(title string, content fyne.CanvasObject, size fyne.Size) *widget.PopUp {
	card := widget.NewCard(title, "", content)
	return widget.NewModalPopUp(container.NewGridWrap(size, card), u.window.Canvas())
}

func (u *updateController) text(key string) string {
	language, systemLanguage := u.language()
	return u.catalog.Text(language, systemLanguage, key)
}
