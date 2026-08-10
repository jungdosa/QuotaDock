package windows

import (
	"errors"
	"fyne.io/fyne/v2"
	"github.com/jungdosa/QuotaDock/internal/i18n"
	"github.com/jungdosa/QuotaDock/internal/security"
	"github.com/jungdosa/QuotaDock/internal/settings"
	"testing"
)

func TestRoundedCornerRadiusScalesWithWindowDPI(t *testing.T) {
	for _, test := range []struct {
		dpi  uint32
		want int
	}{{96, 8}, {120, 10}, {144, 12}, {192, 16}, {0, 8}} {
		if got := scaledCornerRadius(8, test.dpi); got != test.want {
			t.Fatalf("scaledCornerRadius(8, %d)=%d, want %d", test.dpi, got, test.want)
		}
	}
}

func TestNegativeMonitorCoordinatesArePreserved(t *testing.T) {
	monitors := []Rect{{X: 0, Y: 0, Width: 1920, Height: 1080}, {X: -1600, Y: -200, Width: 1600, Height: 900}}
	saved := Rect{X: -1500, Y: -150, Width: 560, Height: 300}
	if got := RestoreRect(saved, monitors); got != saved {
		t.Fatalf("negative coordinates changed: %+v", got)
	}
}
func TestRemovedMonitorPositionReturnsToPrimary(t *testing.T) {
	primary := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	got := RestoreRect(Rect{X: -1500, Y: 30, Width: 560, Height: 300}, []Rect{primary})
	if got.X < 0 || got.Y < 0 || !IsVisible(got, []Rect{primary}) {
		t.Fatalf("off-screen correction=%+v", got)
	}
}

type memoryRunKey struct {
	values map[string]string
	sets   int
}

func (m *memoryRunKey) Get(name string) (string, error) {
	value, ok := m.values[name]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}
func (m *memoryRunKey) Set(name, value string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[name] = value
	m.sets++
	return nil
}
func (m *memoryRunKey) Delete(name string) error { delete(m.values, name); return nil }
func TestAutoStartEnableDisableAndDuplicatePrevention(t *testing.T) {
	key := &memoryRunKey{}
	manager := NewAutoStartManagerWithKey("QuotaDock", `C:\Apps\QuotaDock.exe`, false, key)
	if err := manager.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(false); err != nil {
		t.Fatal(err)
	}
	if key.sets != 1 {
		t.Fatalf("registry writes=%d, want 1", key.sets)
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if _, ok := key.values["QuotaDock"]; ok {
		t.Fatal("run entry remains")
	}
}
// --hidden belongs in the Run entry only when the user asked for it, and
// flipping the choice has to rewrite the entry rather than leave a stale one.
func TestAutoStartRecordsTheMinimizedChoice(t *testing.T) {
	key := &memoryRunKey{}
	manager := NewAutoStartManagerWithKey("QuotaDock", `C:\Apps\QuotaDock.exe`, false, key)

	if err := manager.Enable(false); err != nil {
		t.Fatal(err)
	}
	if got := key.values["QuotaDock"]; got != `"C:\Apps\QuotaDock.exe"` {
		t.Fatalf("visible start command=%q", got)
	}
	if minimized, err := manager.StartsMinimized(); err != nil || minimized {
		t.Fatalf("StartsMinimized=%v err=%v, want false", minimized, err)
	}

	if err := manager.Enable(true); err != nil {
		t.Fatal(err)
	}
	if got := key.values["QuotaDock"]; got != `"C:\Apps\QuotaDock.exe" --hidden` {
		t.Fatalf("minimized start command=%q", got)
	}
	if minimized, err := manager.StartsMinimized(); err != nil || !minimized {
		t.Fatalf("StartsMinimized=%v err=%v, want true", minimized, err)
	}
}

// Installs made before the choice existed carry the always-hidden command.
// Enabled must still report them as registered, or the settings screen would
// show autostart as off for everyone upgrading.
func TestAutoStartRecognisesTheOlderHiddenEntry(t *testing.T) {
	key := &memoryRunKey{values: map[string]string{"QuotaDock": `"C:\Apps\QuotaDock.exe" --hidden`}}
	manager := NewAutoStartManagerWithKey("QuotaDock", `C:\Apps\QuotaDock.exe`, false, key)
	enabled, err := manager.Enabled()
	if err != nil || !enabled {
		t.Fatalf("Enabled=%v err=%v, want true", enabled, err)
	}
}

func TestPortableAutoStartIsRejected(t *testing.T) {
	manager := NewAutoStartManagerWithKey("QuotaDock", `D:\QuotaDock.exe`, true, &memoryRunKey{})
	if !errors.Is(manager.Enable(false), ErrPortableAutoStart) {
		t.Fatal("portable autostart was accepted")
	}
}
func TestTrayRuntimeLocalizationAndCheckedState(t *testing.T) {
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	tray := &Tray{catalog: catalog, language: i18n.English, system: i18n.English, show: fyne.NewMenuItem("", nil), settings: fyne.NewMenuItem("", nil), normal: fyne.NewMenuItem("", nil), compact: fyne.NewMenuItem("", nil), nano: fyne.NewMenuItem("", nil), quit: fyne.NewMenuItem("", nil)}
	tray.apply(settings.ModeNormal, false)
	if tray.show.Label != "Show app" {
		t.Fatalf("English tray label=%q", tray.show.Label)
	}
	tray.language = i18n.Korean
	tray.apply(settings.ModeCompact, false)
	if tray.show.Label != "앱 표시" || !tray.compact.Checked || tray.normal.Checked || tray.nano.Checked {
		t.Fatalf("localized tray state=%q checked=%v", tray.show.Label, tray.compact.Checked)
	}
}
func TestAllowedExternalURLOnly(t *testing.T) {
	if !security.IsAllowedExternalURL("https://openai.com/help") || security.IsAllowedExternalURL("https://evil.invalid/") {
		t.Fatal("external URL allowlist mismatch")
	}
}

func TestWindowCloseHidesAndTrayExitQuits(t *testing.T) {
	hidden, quit := 0, 0
	lifecycle := &Lifecycle{Hide: func() { hidden++ }, Quit: func() { quit++ }}
	lifecycle.CloseRequested()
	if hidden != 1 || quit != 0 {
		t.Fatalf("close hidden=%d quit=%d", hidden, quit)
	}
	lifecycle.ExitRequested()
	lifecycle.ExitRequested()
	if quit != 1 || !lifecycle.Exiting() {
		t.Fatalf("exit quit=%d exiting=%v", quit, lifecycle.Exiting())
	}
}

func TestFullscreenSurfaceRequiresBorderlessFullCover(t *testing.T) {
	monitor := winRect{Left: 0, Top: 0, Right: 3840, Bottom: 2160}
	const caption = 0x00C00000
	tests := []struct {
		name   string
		style  uintptr
		window winRect
		want   bool
	}{
		{"fullscreen chrome video", 0x96000000, winRect{Left: 0, Top: 0, Right: 3840, Bottom: 2160}, true},
		{"borderless overshooting the monitor", 0, winRect{Left: -8, Top: -8, Right: 3848, Bottom: 2168}, true},
		{"maximized window keeps its caption", caption, winRect{Left: -8, Top: -8, Right: 3848, Bottom: 2168}, false},
		{"borderless but smaller than the monitor", 0, winRect{Left: 0, Top: 0, Right: 1920, Bottom: 2160}, false},
		{"borderless on another monitor", 0, winRect{Left: 3840, Top: 0, Right: 5760, Bottom: 1080}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fullscreenSurface(test.style, test.window, monitor); got != test.want {
				t.Fatalf("fullscreenSurface(%#x, %+v) = %t, want %t", test.style, test.window, got, test.want)
			}
		})
	}
}

func TestRectsIntersectNeedsPositiveArea(t *testing.T) {
	monitor := winRect{Left: 1920, Top: 2160, Right: 3840, Bottom: 3240}
	if !rectsIntersect(winRect{Left: 3220, Top: 2160, Right: 3840, Bottom: 2872}, monitor) {
		t.Fatal("overlapping rect was not detected")
	}
	if rectsIntersect(winRect{Left: 0, Top: 0, Right: 1920, Bottom: 2160}, monitor) {
		t.Fatal("edge-touching rect must not count as intersecting")
	}
}

func TestDesktopWindowClassesAreExcluded(t *testing.T) {
	for _, name := range []string{"Progman", "WorkerW", "Windows.UI.Core.CoreWindow"} {
		if !desktopWindowClass(name) {
			t.Fatalf("%s must be excluded from fullscreen detection", name)
		}
	}
	if desktopWindowClass("Chrome_WidgetWin_1") {
		t.Fatal("application classes must stay eligible")
	}
}
