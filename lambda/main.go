package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"log/slog"
	"os"
)

type (
	Request  = events.APIGatewayV2HTTPRequest
	Response = events.APIGatewayV2HTTPResponse
)

type Runtime struct {
	app *handler.App
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})).With("mode", "lambda")
	logger.Info("spawned...")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("failed to load AWS configuration", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("loaded AWS configuration")
	runtime := Runtime{
		app: &handler.App{
			Logger:    logger,
			AwsConfig: &cfg,
			Promoter:  promotion.NewStagePromoter(promotion.DefaultStages),
		},
	}

	lambda.Start(runtime.handleEvent)
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
