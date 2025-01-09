package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/config"
)

var lambdaEnvMapString = map[*string]boundEnvVar[string]{
	&config.Lambda.PayloadType: {
		Name:        "lambda-payload-type",
		Description: "The payload type to expect when running in Lambda mode. Supported values are 'api-gateway-v1', 'api-gateway-v2' and 'lambda-url'",
	},
}
