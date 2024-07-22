package helpers

import (
	"time"

	"golang.org/x/time/rate"
)

var OnceAMinute = onceAMinute()

func onceAMinute() rate.Sometimes {
	return rate.Sometimes{
		Interval: time.Minute,
	}
}
