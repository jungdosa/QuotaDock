// Package fonts embeds the font faces distributed with QuotaDock.
package fonts

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

var (
	//go:embed Pretendard-Regular.ttf
	pretendardRegularData []byte

	//go:embed Pretendard-Bold.ttf
	pretendardBoldData []byte

	pretendardRegular = staticResource("Pretendard-Regular.ttf", pretendardRegularData)
	pretendardBold    = staticResource("Pretendard-Bold.ttf", pretendardBoldData)
)

// PretendardRegular returns the embedded regular face, or nil if its data is unavailable.
func PretendardRegular() fyne.Resource { return pretendardRegular }

// PretendardBold returns the embedded bold face, or nil if its data is unavailable.
func PretendardBold() fyne.Resource { return pretendardBold }

func staticResource(name string, data []byte) fyne.Resource {
	if len(data) == 0 {
		return nil
	}
	return fyne.NewStaticResource(name, data)
}
