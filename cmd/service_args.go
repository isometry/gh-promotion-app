package cmd

import (
	"time"

	"github.com/isometry/gh-promotion-app/internal/helpers"
)

var svcEnvMapString = map[*string]boundEnvVar[string]{
	&svcHostAddr: {
		Name:        "addr",
		Description: "The address to serve the service on (default all interfaces in dual-stack runtimeMode)",
		Short:       helpers.Ptr("H"),
		Default:     helpers.Ptr(""),
	},
	&svcHostPort: {
		Name:        "port",
		Description: "The port to serve the service on",
		Short:       helpers.Ptr("p"),
		Default:     helpers.Ptr("8080"),
	},
	&svcHostPath: {
		Name:        "path",
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
		Default:     helpers.Ptr(5 * time.Second),
	},
}
