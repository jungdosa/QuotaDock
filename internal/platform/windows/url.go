package windows

import (
	"errors"
	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/security"
	"net/url"
)

var ErrURLNotAllowed = errors.New("external URL is not on the official allowlist")

func OpenAllowedURL(app fyne.App, raw string) error {
	if !security.IsAllowedExternalURL(raw) {
		return ErrURLNotAllowed
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ErrURLNotAllowed
	}
	return app.OpenURL(parsed)
}
