package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/isometry/gh-promotion-app/internal/handler"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

type Request = events.APIGatewayV2HTTPRequest
type Response = events.APIGatewayV2HTTPResponse

type Runtime struct {
	app handler.App
}

func (r *Runtime) handleEvent(ctx context.Context, request Request) (response Response, err error) {
	r.app.Logger.Info("received API Gateway V2 request")

	hResponse, err := r.app.HandleEvent(ctx, handler.Request{
		Body:    request.Body,
		Headers: request.Headers,
	})
	r.app.Logger.Info("handled event", slog.Any("response", hResponse), slog.Any("error", err))
	return Response{
		Body:       hResponse.Body,
		StatusCode: hResponse.StatusCode,
	}, err
}

func main() {
	ctx := context.TODO()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	logger.Info("spawned lambda")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Warn("failed to load AWS configuration", slog.Any("error", err))
		panic(err)
	}

	logger.Info("loaded AWS configuration")

	runtime := Runtime{
		app: handler.App{
			Logger:    logger,
			AwsConfig: &cfg,
		},
	}

	lambda.Start(runtime.handleEvent)
}
