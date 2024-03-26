package cmd

import (
	"context"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var lambdaCmd = &cobra.Command{
	Use:     "lambda",
	Aliases: []string{"l", "serverless"},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		logger = logger.With("mode", "lambda")
		logger.Info("spawning...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var promoter *promotion.Promoter
		if !dynamicPromoter {
			promoter = promotion.NewDefaultPromoter()
		} else {
			logger.Info("dynamic promoter activated...")
		}
		logger.Debug("creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithAuthMode(githubAuthMode),
			handler.WithSSMKey(githubSSMKey),
			handler.WithToken(githubToken),
			handler.WithWebhookSecret(webhookSecret), // @TODO -> This breaks in lambda, we need to fetch it from SSM
			handler.WithPromoter(promoter),
			handler.WithDynamicPromoterKey(dynamicPromoterKey),
			handler.WithContext(cmd.Context()),
			handler.WithLogger(logger.With("component", "promotion-handler")))
		if err != nil {
			return errors.Wrap(err, "failed to create promotion handler")
		}

		logger.Debug("creating runtime...")
		runtime := runtime.NewRuntime(hdl,
			runtime.WithLogger(logger.With("component", "runtime")))

		logger.Info("lambda starting...")
		lambda.StartWithOptions(runtime.HandleEvent,
			lambda.WithContext(context.Background()))

		return nil
	},
}
