package helpers

import "time"

func TimeDurationPtr(duration time.Duration) *time.Duration {
	return &duration
}
