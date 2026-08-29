package main

import platform "github.com/jungdosa/QuotaDock/internal/platform/windows"

// Rectangles reach the diagnostic log as plain integer arrays. The log writer
// only accepts values it can serialize, so window geometry is flattened here
// rather than handed over as a struct.

func rectValue(rect platform.Rect) []int {
	return []int{rect.X, rect.Y, rect.Width, rect.Height}
}

func areasValue(areas []platform.Rect) [][]int {
	value := make([][]int, 0, len(areas))
	for _, area := range areas {
		value = append(value, rectValue(area))
	}
	return value
}
