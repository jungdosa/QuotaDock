//go:build !windows

package windows

func SupportsTrayIconPromotion() bool {
	return false
}

func SetTrayIconPromoted(string, bool) error {
	return nil
}
