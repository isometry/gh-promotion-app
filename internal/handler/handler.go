package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/google/go-github/v59/github"

	"github.com/isometry/gh-promotion-app/internal/ghapp"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

var HandledEventTypes = []string{
	"push", // triggers promotion request creation
	// all other events trigger promotion request fast forward
	"pull_request",
	"pull_request_review",
	"check_suite",
	"deployment_status",
	"status",
}

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

type EventInstallationId struct {
	Installation struct {
		ID *int64 `json:"id"`
	} `json:"installation"`
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
	slog.Info("handling request")

	eventType := request.Headers[strings.ToLower(github.EventTypeHeader)]
	if eventType == "" {
		slog.Warn("ERROR: missing event type")
		return Response{Body: "missing event type", StatusCode: 400}, nil
	}
	slog.Info("found event type", slog.String("eventType", eventType))

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
		bucket := os.Getenv("S3_BUCKET_NAME")
		if bucket != "" {
			// configure S3 client if it is not already set
			if app.s3Client == nil {
				app.s3Client = s3.NewFromConfig(*app.AwsConfig)
			}

			key := fmt.Sprintf("%s.%s", time.Now().UTC().Format(time.RFC3339Nano), eventType)
			_, err = app.s3Client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      &bucket,
				Key:         aws.String(key),
				Body:        strings.NewReader(request.Body),
				ContentType: aws.String("application/json"),
			})
			if err != nil {
				slog.Error("ERROR: failed to store request object", slog.Any("error", err))
			}
		}
	}

	if slices.Index(HandledEventTypes, eventType) == -1 {
		slog.Info("ignoring event", slog.String("eventType", eventType))
		return Response{StatusCode: 204}, nil
	}

	var EventInstallationId EventInstallationId
	err = json.Unmarshal([]byte(request.Body), &EventInstallationId)
	if err != nil {
		slog.Warn("No installation ID found in event", slog.Any("error", err), slog.String("eventType", eventType))
		return Response{StatusCode: 204}, nil
	}
	installationId := EventInstallationId.Installation.ID

	// XXX: could/should we cache the installation client? …token TTL is only 1 hour
	client, err := app.ghAppCreds.GetInstallationClient(*installationId)
	if err != nil {
		slog.Error("Failed to get installation client", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: 500}, nil
	}
	slog.Info("Got installation client")
	pCtx.Client = client

	switch e := event.(type) {
	case *github.PushEvent:
		slog.Info("Received push event")

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

		if requestUrl, _ := pCtx.FindRequest(); requestUrl != nil {
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

	case *github.PullRequestEvent:
		slog.Info("Received pull request event")

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

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.StandardRef(e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.StandardRef(e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

	case *github.CheckSuiteEvent:
		slog.Info("Received check suite event")

		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			slog.Info("Ignoring incomplete check suite event", slog.String("status", *e.CheckSuite.Status))
			return Response{StatusCode: 204}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && promotion.IsPromotionRequest(pr) {
				pCtx.BaseRef = pr.Base.Ref
				pCtx.HeadRef = pr.Head.Ref
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			slog.Info("Ignoring check suite event without matching promotion request", slog.String("headSHA", *pCtx.HeadSHA))
			return Response{StatusCode: 204}, nil
		}

	case *github.DeploymentStatusEvent:
		slog.Info("Received deployment status event")

		if *e.DeploymentStatus.State != "success" {
			slog.Info("Ignoring non-success deployment status event", slog.String("state", *e.DeploymentStatus.State))
			return Response{StatusCode: 204}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = helpers.StandardRef(e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

	case *github.StatusEvent:
		slog.Info("Received status event")

		if *e.State != "success" {
			slog.Info("Ignoring non-success status event", slog.String("state", *e.State))
			return Response{StatusCode: 204}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.SHA

	default:
		slog.Warn("Unhandled event type", slog.String("eventType", eventType), slog.Any("event", e))
		return Response{StatusCode: 204}, nil
	}

	// ignore events on non-promotion heads
	if !promotion.IsPromoteableRef(*pCtx.HeadRef) {
		slog.Info("Ignoring event on non-promotion head ref", slog.String("event", eventType), slog.String("headRef", *pCtx.HeadRef))
		return Response{StatusCode: 204}, nil
	}
	slog.Info("Event relates to promotion ref", slog.String("headRef", *pCtx.HeadRef))

	// ignore events without an open promotion request
	if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
		// find matching promotion request by head SHA and populate missing refs
		if _, err = pCtx.FindRequest(); err != nil {
			slog.Error("Failed to find promotion request", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return Response{StatusCode: 500}, nil
		}
	}

	slog.Info("attempting fast forward")
	if err = pCtx.FastForwardRefToSha(ctx); err != nil {
		slog.Error("Failed to fast forward", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: 500}, nil
	}
	slog.Info("fast forward complete", slog.String("headRef", *pCtx.HeadRef), slog.String("baseRef", *pCtx.BaseRef), slog.String("headSHA", *pCtx.HeadSHA))
	return Response{Body: "promotion complete", StatusCode: 201}, nil
}
