package main

import (
	"context"
	"log/slog"

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
	slog.Info("Received API Gateway V2 request")

	hResponse, err := r.app.HandleEvent(ctx, handler.Request{
		Body:    request.Body,
		Headers: request.Headers,
	})
	return Response{
		Body:       hResponse.Body,
		StatusCode: hResponse.StatusCode,
	}, err
}

func main() {
	ctx := context.TODO()
	slog.Info("spawned lambda")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Warn("failed to load AWS configuration", slog.Any("error", err))
		panic(err)
	}

	slog.Info("loaded AWS configuration")

	runtime := Runtime{
		app: handler.App{
			AwsConfig: &cfg,
		},
	}

	lambda.Start(runtime.handleEvent)
}
