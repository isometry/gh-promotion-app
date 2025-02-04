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

var (
	promotionRuntime *runtime.Runtime
)

var lambdaCmd = &cobra.Command{
	Use: "lambda",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cmd.Parent().Parent().PersistentPreRun(cmd, args)
		return nil
	},
}

// lambdaHTTPCmd is the command for running the lambda-http mode.
var lambdaHTTPCmd = &cobra.Command{
	Use: "http",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := setup(cmd); err != nil {
			return errors.Wrap(err, "failed to setup lambda")
		}

		logger = logger.With("mode", config.Global.Mode)
		logger.Info("lambda starting...")
		lambda.StartWithOptions(promotionRuntime.Lambda,
			lambda.WithContext(cmd.Context()))

		return nil
	},
}

// lambdaEventCmd is the command for running the lambda in event mode.
var lambdaEventCmd = &cobra.Command{
	Use: "event",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := setup(cmd); err != nil {
			return errors.Wrap(err, "failed to setup lambda")
		}

		logger = logger.With("mode", config.Global.Mode)

		logger.Info("lambda starting...")
		lambda.StartWithOptions(promotionRuntime.LambdaForEvent,
			lambda.WithContext(cmd.Context()))
		return nil
	},
}

func init() {
	lambdaCmd.AddCommand(lambdaEventCmd)
	lambdaCmd.AddCommand(lambdaHTTPCmd)

	bindEnvMap(lambdaCmd, lambdaEnvMapString)
}

func setup(cmd *cobra.Command) error {
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
	promotionRuntime = runtime.NewRuntime(hdl,
		runtime.WithLogger(logger.With("component", "runtime")))
	return nil
}
