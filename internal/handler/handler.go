package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/google/go-github/v57/github"
	"github.com/isometry/gh-promotion-app/internal/ghapp"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type App struct {
	AwsConfig  *aws.Config
	s3Client   *s3.Client
	ghAppCreds *ghapp.GHAppCredentials
}

type Request struct {
	Body    string
	Headers map[string]string // lowercase keys to match AWS Lambda proxy request
}

type Response struct {
	Body       string
	StatusCode int
}

func (app *App) RetrieveGhAppCreds(ctx context.Context) error {
	slog.Info("Retrieving GitHub App credentials")

	switch {
	case app.AwsConfig != nil:
		return app.RetrieveGhAppCredsFromSSM(ctx)
	}
	return fmt.Errorf("no GitHub App credentials available")
}

func (app *App) RetrieveGhAppCredsFromSSM(ctx context.Context) error {
	slog.Info("Retrieving GitHub App credentials from SSM")

	ssm_client := ssm.NewFromConfig(*app.AwsConfig)

	ssmResponse, err := ssm_client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(os.Getenv("GITHUB_APP_SSM_ARN")),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		slog.Warn("failed to load SSM parameters", slog.Any("error", err))
		return err
	}

	ghaParams := []byte(*ssmResponse.Parameter.Value)

	var ghAppCreds ghapp.GHAppCredentials
	if err = json.Unmarshal(ghaParams, &ghAppCreds); err != nil {
		slog.Warn("failed to unmarshal SSM parameters", slog.Any("error", err))
		return err
	}

	app.ghAppCreds = &ghAppCreds

	return nil
}

func (app *App) RetrieveGhAppCredsFromVault(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (app *App) HandleEvent(ctx context.Context, request Request) (response Response, err error) {
	slog.Info("Received request")

	eventType := request.Headers[strings.ToLower(github.EventTypeHeader)]
	if eventType == "" {
		slog.Warn("ERROR: missing event type")
		return Response{Body: "missing event type", StatusCode: 400}, nil
	}
	slog.Info("Received event type", slog.String("eventType", eventType))

	// retrieve GitHub App credentials if they are not already set
	if app.ghAppCreds == nil {
		err = app.RetrieveGhAppCreds(ctx)
		if err != nil {
			slog.Warn("ERROR: failed to retrieve GitHub App credentials", slog.Any("error", err))
			return Response{StatusCode: 500}, nil
		}
	}

	err = app.ghAppCreds.ValidateSignature([]byte(request.Body), request.Headers)
	if err != nil {
		slog.Warn("ERROR: validating signature", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: 401}, nil
	}
	slog.Info("Request is valid")

	// request is valid

	event, err := github.ParseWebHook(eventType, []byte(request.Body))
	if err != nil {
		slog.Error("ERROR: parsing payload", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: 400}, nil
	}

	pCtx := promotion.Context{
		EventType: &eventType,
	}

	if app.AwsConfig != nil {
		// configure S3 client if it is not already set
		if app.s3Client == nil {
			app.s3Client = s3.NewFromConfig(*app.AwsConfig)
		}

		key := fmt.Sprintf("%s.%s", time.Now().UTC().Format(time.RFC3339Nano), eventType)
		_, err = app.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(os.Getenv("BUCKET_NAME")),
			Key:         aws.String(key),
			Body:        strings.NewReader(request.Body),
			ContentType: aws.String("application/json"),
		})
		if err != nil {
			slog.Error("ERROR: failed to store request object", slog.Any("error", err))
		}
	}

	var installationId *int64

	switch e := event.(type) {
	case *github.PingEvent:
		slog.Info("Received ping event")
		return Response{StatusCode: 204}, nil

	case *github.InstallationEvent:
		slog.Info("Received installation event")
		return Response{StatusCode: 204}, nil

	case *github.PushEvent:
		slog.Info("Received push event")

		installationId = e.Installation.ID
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = e.Ref // already fully qualified
		pCtx.HeadSHA = e.After

		if stageIndex := promotion.StageIndex(*e.Ref); stageIndex != -1 && stageIndex < len(promotion.Stages)-1 {
			baseRef := promotion.StageRef(promotion.Stages[stageIndex+1])
			pCtx.BaseRef = &baseRef
		} else {
			slog.Info("Ignoring push event on non-promotion branch", slog.String("ref", *pCtx.HeadRef), slog.String("sha", *pCtx.HeadSHA))
			return Response{StatusCode: 204}, nil
		}

	case *github.DeploymentStatusEvent:
		slog.Info("Received deployment status event")

		if *e.DeploymentStatus.State != "success" {
			slog.Info("Ignoring non-success deployment status event", slog.String("state", *e.DeploymentStatus.State))
			return Response{StatusCode: 204}, nil
		}

		installationId = e.Installation.ID
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = helpers.StandardRef(e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		// TODO: find base ref from deployment status event?

	case *github.StatusEvent:
		slog.Info("Received status event")

		if *e.State != "success" {
			slog.Info("Ignoring non-success status event", slog.String("state", *e.State))
			return Response{StatusCode: 204}, nil
		}

		installationId = e.Installation.ID
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.SHA

	case *github.PullRequestEvent:
		slog.Info("Received pull request event")

		installationId = e.Installation.ID
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.StandardRef(e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.StandardRef(e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

	case *github.PullRequestReviewEvent:
		slog.Info("Received pull request review event")

		if *e.Review.State != "approved" {
			slog.Info("Ignoring non-approved pull request review event", slog.String("state", *e.Review.State))
			return Response{StatusCode: 204}, nil
		}

		installationId = e.Installation.ID
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.StandardRef(e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.StandardRef(e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

	default:
		slog.Warn("Unhandled event type", slog.String("eventType", eventType), slog.Any("event", e))
		return Response{StatusCode: 204}, nil
	}

	if installationId == nil {
		slog.Warn("No installation ID found in event")
		return Response{StatusCode: 204}, nil
	}

	// ignore events on non-promotion heads
	if !promotion.IsPromoteableRef(*pCtx.HeadRef) {
		slog.Info("Ignoring event on non-promotion head ref", slog.String("event", eventType), slog.String("headRef", *pCtx.HeadRef))
		return Response{StatusCode: 204}, nil
	}
	slog.Info("Event relates to promotion ref", slog.String("headRef", *pCtx.HeadRef))

	// XXX: could/should we cache the installation client?
	client, err := app.ghAppCreds.GetInstallationClient(*installationId)
	if err != nil {
		slog.Error("Failed to get installation client", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: 500}, nil
	}
	slog.Info("Got installation client")
	pCtx.Client = client

	// only push events trigger promotion request creation
	switch eventType {
	case "push":
		slog.Info("Checking for existing promotion request")
		if pr, _ := pCtx.FindRequest(); pr != nil {
			// PR already exists covering this push event
			slog.Info("Existing promotion request found, skipping creation")
			return Response{StatusCode: 204}, nil
		}

		slog.Info("Creating promotion request")
		pr, err := pCtx.CreateRequest()
		if err != nil {
			slog.Error("Failed to create promotion request", slog.Any("error", err))
			return Response{Body: err.Error(), StatusCode: 500}, nil
		}
		return Response{Body: pr.GetURL(), StatusCode: 201}, nil

	case "pull_request":
		fallthrough
	case "pull_request_review":
		slog.Info("attempting fast forward")
		err = pCtx.FastForwardRefToSha(ctx)
		if err != nil {
			slog.Error("Failed to fast forward", slog.Any("error", err))
			return Response{Body: err.Error(), StatusCode: 500}, nil
		}
		return Response{StatusCode: 201}, nil
	}

	return Response{StatusCode: 204}, nil
}
