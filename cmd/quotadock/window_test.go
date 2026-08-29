package main

import (
	"reflect"
	"testing"

	platform "github.com/jungdosa/QuotaDock/internal/platform/windows"
)

func TestRectValueKeepsFieldOrder(t *testing.T) {
	// The order is what the log reader relies on; a swap here would be silent.
	got := rectValue(platform.Rect{X: -1920, Y: 40, Width: 1728, Height: 1488})
	if want := []int{-1920, 40, 1728, 1488}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rectValue = %v, want %v", got, want)
	}
}

func TestAreasValueHandlesEmptySingleAndMultiple(t *testing.T) {
	if got := areasValue(nil); !reflect.DeepEqual(got, [][]int{}) {
		t.Fatalf("areasValue(nil) = %v, want an empty slice, not nil", got)
	}
	single := areasValue([]platform.Rect{{X: 0, Y: 0, Width: 1920, Height: 1032}})
	if want := [][]int{{0, 0, 1920, 1032}}; !reflect.DeepEqual(single, want) {
		t.Fatalf("areasValue single = %v, want %v", single, want)
	}
	multiple := areasValue([]platform.Rect{
		{X: 0, Y: 0, Width: 1920, Height: 1032},
		{X: -1920, Y: 0, Width: 1920, Height: 1032},
		{X: -4080, Y: 0, Width: 1728, Height: 1488},
	})
	want := [][]int{{0, 0, 1920, 1032}, {-1920, 0, 1920, 1032}, {-4080, 0, 1728, 1488}}
	if !reflect.DeepEqual(multiple, want) {
		t.Fatalf("areasValue multiple = %v, want %v", multiple, want)
	}
}
