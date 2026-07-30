// Package windows owns Windows-only desktop behavior and its testable geometry rules.
package windows

import "errors"

type Rect struct{ X, Y, Width, Height int }

func (r Rect) Right() int  { return r.X + r.Width }
func (r Rect) Bottom() int { return r.Y + r.Height }
func (r Rect) Valid() bool { return r.Width > 0 && r.Height > 0 }
func intersection(a, b Rect) Rect {
	x := max(a.X, b.X)
	y := max(a.Y, b.Y)
	right := min(a.Right(), b.Right())
	bottom := min(a.Bottom(), b.Bottom())
	if right <= x || bottom <= y {
		return Rect{}
	}
	return Rect{X: x, Y: y, Width: right - x, Height: bottom - y}
}
func IsVisible(window Rect, monitors []Rect) bool {
	for _, m := range monitors {
		i := intersection(window, m)
		if i.Width >= 32 && i.Height >= 32 {
			return true
		}
	}
	return false
}
func RestoreRect(saved Rect, monitors []Rect) Rect {
	if saved.Width <= 0 {
		saved.Width = 560
	}
	if saved.Height <= 0 {
		saved.Height = 300
	}
	if IsVisible(saved, monitors) {
		return saved
	}
	if len(monitors) == 0 {
		return Rect{X: 0, Y: 0, Width: saved.Width, Height: saved.Height}
	}
	primary := monitors[0]
	width := min(saved.Width, primary.Width)
	height := min(saved.Height, primary.Height)
	return Rect{X: primary.X + (primary.Width-width)/2, Y: primary.Y + (primary.Height-height)/2, Width: width, Height: height}
}

var ErrPortableAutoStart = errors.New("automatic start is disabled for portable builds")
