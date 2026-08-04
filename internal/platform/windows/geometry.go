// Package windows owns Windows-only desktop behavior and its testable geometry rules.
package windows

import (
	"cmp"
	"errors"
	"slices"
)

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

func WorkAreasEqual(left, right []Rect) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]Rect(nil), left...)
	right = append([]Rect(nil), right...)
	order := func(a, b Rect) int {
		if value := cmp.Compare(a.X, b.X); value != 0 {
			return value
		}
		if value := cmp.Compare(a.Y, b.Y); value != 0 {
			return value
		}
		if value := cmp.Compare(a.Width, b.Width); value != 0 {
			return value
		}
		return cmp.Compare(a.Height, b.Height)
	}
	slices.SortFunc(left, order)
	slices.SortFunc(right, order)
	return slices.Equal(left, right)
}

func overlapsWorkArea(window Rect, areas []Rect) bool {
	for _, area := range areas {
		if intersection(window, area).Valid() {
			return true
		}
	}
	return false
}

// DisplayChange decides whether a changed monitor configuration requires the
// existing FitToWorkArea safety net. A window with any overlap is left alone.
func DisplayChange(previous, current []Rect, window Rect) (changed bool, fitted Rect, shouldFit bool) {
	if WorkAreasEqual(previous, current) {
		return false, window, false
	}
	if overlapsWorkArea(window, current) {
		return true, window, false
	}
	fitted = FitToWorkArea(window, current)
	return true, fitted, fitted != window
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

// FitToWorkArea moves a window fully inside the work area with which it has
// the largest overlap. It never changes the window size.
func FitToWorkArea(window Rect, areas []Rect) Rect {
	if len(areas) == 0 {
		return window
	}

	area := areas[0]
	var largestOverlap int64
	for _, candidate := range areas {
		overlap := intersection(window, candidate)
		overlapArea := int64(overlap.Width) * int64(overlap.Height)
		if overlapArea > largestOverlap {
			largestOverlap = overlapArea
			area = candidate
		}
	}

	fitted := window
	if fitted.Right() > area.Right() {
		fitted.X = area.Right() - fitted.Width
	}
	if fitted.X < area.X {
		fitted.X = area.X
	}
	if fitted.Bottom() > area.Bottom() {
		fitted.Y = area.Bottom() - fitted.Height
	}
	if fitted.Y < area.Y {
		fitted.Y = area.Y
	}
	return fitted
}

var ErrPortableAutoStart = errors.New("automatic start is disabled for portable builds")
