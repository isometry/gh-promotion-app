package cmd

import (
	"os"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	promotionRuntime *runtime.Runtime
)

func cmdLambda() *cobra.Command {
	cmd := &cobra.Command{
		Use: "lambda",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			parent := cmd.Parent()
			for parent != nil {
				if parent.PersistentPreRun != nil {
					parent.PersistentPreRun(cmd, args)
				}
				parent = parent.Parent()
			}
			return setup(cmd)
		},
	}

	cmd.AddCommand(
		cmdLambdaHTTP(),
		cmdLambdaEvent(),
	)

	bindEnvMap(cmd, lambdaEnvMapString)

	return cmd
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
