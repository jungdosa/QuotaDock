//go:build !windows

package windows

func CurrentWindowsBuild() (int, error) {
	return 0, errUnsupportedPlatform
}

func SupportsTrayIconPromotion() bool {
	return false
}

func SetTrayIconPromoted(string, bool) (TrayPromotionResult, error) {
	return TrayPromotionUnsupported, nil
}
