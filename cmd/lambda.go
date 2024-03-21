package cmd

import (
	"context"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/isometry/gh-promotion-app/cmd/helpers"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var lambdaCmd = &cobra.Command{
	Use:     "lambda",
	Aliases: []string{"l", "serverless"},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		logger = logger.With("mode", "lambda")
		logger.Info("spawned...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		hdl, err := handler.NewPromotionHandler()
		if err != nil {
			return errors.Wrap(err, "failed to create promotion handler")
		}

		runtime := helpers.Runtime{Handler: hdl}

		lambda.StartWithOptions(runtime.HandleEvent,
			lambda.WithContext(context.Background()))

		return nil
	},
}
