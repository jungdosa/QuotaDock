//go:build !windows

package windows

func SupportsTrayIconPromotion() bool {
	return false
}

func SetTrayIconPromoted(string, bool) (TrayPromotionResult, error) {
	return TrayPromotionUnsupported, nil
}
