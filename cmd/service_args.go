package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"time"
)

var svcEnvMapString = map[*string]boundEnvVar[string]{
	&svcHostAddr: {
		Name:        "service-host-addr",
		Description: "The address to serve the service on (default all interfaces in dual-stack serviceMode)",
		Short:       helpers.Ptr("H"),
		Default:     helpers.Ptr(""),
	},
	&svcHostPort: {
		Name:        "service-host-port",
		Description: "The port to serve the service on",
		Short:       helpers.Ptr("p"),
		Default:     helpers.Ptr("8080"),
	},
	&svcHostPath: {
		Name:        "service-host-path",
		Description: "The path to serve the service on",
		Short:       helpers.Ptr("P"),
		Default:     helpers.Ptr("/"),
	},
}

var svcEnvMapDuration = map[*time.Duration]boundEnvVar[time.Duration]{
	&svcIoTimeout: {
		Name:        "service-io-timeout",
		Description: "The timeout for I/O operations",
		Short:       helpers.Ptr("t"),
		Default:     helpers.TimeDurationPtr(5 * time.Second),
	},
}
