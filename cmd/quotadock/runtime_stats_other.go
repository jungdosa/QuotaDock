//go:build !windows

package main

func processCPUSeconds() (float64, bool) { return 0, false }
