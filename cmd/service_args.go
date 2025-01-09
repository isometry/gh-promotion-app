package cmd

import (
	"time"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/helpers"
)

var svcEnvMapString = map[*string]boundEnvVar[string]{
	&config.Service.Addr: {
		Name:        "addr",
		Description: "The address to serve the service on (default all interfaces in dual-stack mode)",
		Short:       helpers.Ptr("H"),
	},
	&config.Service.Port: {
		Name:        "port",
		Description: "The port to serve the service on",
		Short:       helpers.Ptr("p"),
	},
	&config.Service.Path: {
		Name:        "host-path",
		Description: "The host-path to serve the service on",
		Short:       helpers.Ptr("P"),
	},
}

var svcEnvMapDuration = map[*time.Duration]boundEnvVar[time.Duration]{
	&config.Service.Timeout: {
		Name:        "service-io-timeout",
		Description: "The timeout for I/O operations",
		Short:       helpers.Ptr("t"),
	},
}
