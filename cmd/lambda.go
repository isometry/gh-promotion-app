package cmd

import (
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var lambdaCmd = &cobra.Command{
	Use:     "lambda",
	Aliases: []string{"l", "serverless"},
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger = logger.With("mode", "lambda")
		logger.Debug("creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithLambdaPayloadType(config.Lambda.PayloadType),
			handler.WithAuthMode(config.GitHub.AuthMode),
			handler.WithSSMKey(config.GitHub.SSMKey),
			handler.WithToken(os.Getenv("GITHUB_TOKEN")),
			handler.WithWebhookSecret(config.GitHub.WebhookSecret),
			handler.WithContext(cmd.Context()),
			handler.WithLogger(logger))
		if err != nil {
			return errors.Wrap(err, "failed to create promotion handler")
		}

		logger.Debug("creating runtime...")
		runtime := runtime.NewRuntime(hdl,
			runtime.WithLogger(logger.With("component", "runtime")))

		logger.Info("lambda starting...")
		lambda.StartWithOptions(runtime.Lambda,
			lambda.WithContext(cmd.Context()))

		return nil
	},
}

func init() {
	bindEnvMap(lambdaCmd, lambdaEnvMapString)
}
