//go:build !windows

package webview

// DetectRuntime always reports absent off Windows: the embedded auth browser
// is WebView2, which only exists there. macOS will grow a WKWebView probe when
// that platform lands.
func DetectRuntime() Availability { return Availability{} }
