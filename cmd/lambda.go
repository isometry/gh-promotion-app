package cmd

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	lambdaPayloadType string
)

var lambdaCmd = &cobra.Command{
	Use:     "lambda",
	Aliases: []string{"l", "serverless"},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		loadViperVariables(cmd)
		logger = logger.With("mode", "lambda")
		logger.Info("spawning...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Debug("creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithLambdaPayloadType(lambdaPayloadType),
			handler.WithAuthMode(githubAuthMode),
			handler.WithSSMKey(githubSSMKey),
			handler.WithToken(githubToken),
			handler.WithWebhookSecret(webhookSecret),
			handler.WithDynamicPromotion(dynamicPromotion),
			handler.WithDynamicPromotionKey(dynamicPromotionKey),
			handler.WithFeedbackCommitStatus(feedbackCommitStatus),
			handler.WithFeedbackCommitStatusContext(feedbackCommitStatusContext),
			handler.WithCreateTargetRef(createTargetRef),
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
