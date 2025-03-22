package cmd

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/spf13/cobra"
)

func cmdLambdaEvent() *cobra.Command {
	cmd := &cobra.Command{
		Use: "event",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger.Info("lambda starting...")
			lambda.StartWithOptions(promotionRuntime.LambdaForEvent,
				lambda.WithContext(cmd.Context()))
			return nil
		},
	}

	return cmd
}
