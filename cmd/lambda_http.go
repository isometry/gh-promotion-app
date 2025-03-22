package cmd

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/spf13/cobra"
)

func cmdLambdaHTTP() *cobra.Command {
	// cmd is the command for running the lambda-http mode.
	cmd := &cobra.Command{
		Use: "http",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger.Info("lambda starting...")
			lambda.StartWithOptions(promotionRuntime.Lambda,
				lambda.WithContext(cmd.Context()))

			return nil
		},
	}

	return cmd
}
