//go:build !windows

package main

import (
	"context"
	"time"
)

func querySystemEvents(context.Context, time.Time, time.Time) ([]systemEvent, error) {
	return nil, nil
}
