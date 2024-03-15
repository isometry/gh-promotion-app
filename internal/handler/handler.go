package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/google/go-github/v60/github"

	"github.com/isometry/gh-promotion-app/internal/ghapp"
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
	"workflow_run",
}

type App struct {
	Logger     *slog.Logger
	AwsConfig  *aws.Config
	Promoter   *promotion.Promoter
	s3Client   *s3.Client
	GhAppCreds *ghapp.GHAppCredentials
	hmacSecret []byte
	ghClient   *github.Client
	ghQLClient *githubv4.Client
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

func (app *App) RetrieveClientCredsFromApp(ctx context.Context, request Request) error {
	app.Logger.Info("retrieving GitHub App credentials")

	if app.AwsConfig != nil {
		return app.RetrieveGhAppCredsFromSSM(ctx)
	}

	var eventInstallationId EventInstallationId
	err := json.Unmarshal([]byte(request.Body), &eventInstallationId)
	if err != nil {
		return fmt.Errorf("no installation ID found. error: %v", err)
	}
	app.GhAppCreds.InstallationId = *eventInstallationId.Installation.ID
	return fmt.Errorf("no GitHub App credentials available")
}

func (app *App) RetrieveClientCredsFromEnv() error {
	webhookSecret := validation.WebhookSecret(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	if webhookSecret == "" {
		return fmt.Errorf("no GitHub App credentials available. [GITHUB_WEBHOOK_SECRET] is not set")
	}
	app.GhAppCreds = &ghapp.GHAppCredentials{
		Token: os.Getenv("GITHUB_TOKEN"),
	}
	app.GhAppCreds.WebhookSecret = &webhookSecret
	return nil
}

func (app *App) RetrieveGhAppCredsFromSSM(ctx context.Context) error {
	app.Logger.Info("Retrieving GitHub App credentials from SSM")

	ssmClient := ssm.NewFromConfig(*app.AwsConfig)

	ssmResponse, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(os.Getenv("GITHUB_APP_SSM_ARN")),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		app.Logger.Warn("failed to load SSM parameters", slog.Any("error", err))
		return err
	}

	ghaParams := []byte(*ssmResponse.Parameter.Value)

	if err = json.Unmarshal(ghaParams, &app.GhAppCreds); err != nil {
		app.Logger.Warn("failed to unmarshal SSM parameters", slog.Any("error", err))
		return err
	}

	return nil
}

func (app *App) RetrieveGhAppCredsFromVault(ctx context.Context) error {
	panic("not implemented")
}

func (app *App) HandleEvent(ctx context.Context, request Request) (response Response, err error) {
	logger := app.Logger

	logger.Info("handling request")

	eventType := request.Headers[strings.ToLower(github.EventTypeHeader)]
	if eventType == "" {
		logger.Warn("missing event type")
		return Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing event type")
	}

	if slices.Index(HandledEventTypes, eventType) == -1 {
		logger.Info("unhandled event")
		return Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("unhandled event type: %s", eventType)
	}

	// if a GITHUB_TOKEN is set, use it to create a GitHub client
	if os.Getenv("GITHUB_TOKEN") != "" {
		logger.Info("using GITHUB_TOKEN to create GitHub client")
		if err = app.RetrieveClientCredsFromEnv(); err != nil {
			logger.Warn("retrieving GitHub credentials", slog.Any("error", err))
			return Response{StatusCode: http.StatusInternalServerError}, fmt.Errorf("unable to retrieve GitHub App credentials from environment")
		}
	}

	// retrieve GitHub App credentials if they are not already set
	if app.GhAppCreds == nil {
		if err = app.RetrieveClientCredsFromApp(ctx, request); err != nil {
			logger.Warn("retrieving GitHub App credentials", slog.Any("error", err))
			return Response{StatusCode: http.StatusInternalServerError}, fmt.Errorf("unable to retrieve GitHub App credentials")
		}
	}

	if err = app.GhAppCreds.WebhookSecret.ValidateSignature([]byte(request.Body), request.Headers); err != nil {
		logger.Error("validating signature", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusForbidden}, err
	}
	logger.Debug("request is valid")

	event, err := github.ParseWebHook(eventType, []byte(request.Body))
	if err != nil {
		logger.Warn("parsing webhook payload", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("invalid payload")
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
				logger.Error("logging request", slog.Any("error", err))
			}
		}
	}

	// XXX: could/should we cache the installation client? …token TTL is only 1 hour
	app.ghClient, app.ghQLClient, err = app.GhAppCreds.GetClient()
	if err != nil {
		return Response{StatusCode: http.StatusInternalServerError}, errors.Wrap(err, "failed to create GitHub client")
	}

	pCtx := promotion.Context{
		EventType:     &eventType,
		Logger:        logger.With("routine", "promotion.Context"),
		Client:        app.ghClient,
		ClientGraphQL: app.ghQLClient,
		Promoter:      app.Promoter,
	}

	logger = logger.With(slog.Any("context", pCtx))
	switch e := event.(type) {
	case *github.PushEvent:
		logger.Debug("processing push event...")

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = e.Ref // already fully qualified
		pCtx.HeadSHA = e.After

		if nextStage, isPromotable := app.Promoter.IsPromotableRef(*e.Ref); isPromotable {
			pCtx.BaseRef = promotion.StageRef(nextStage)
		} else {
			msg := "ignoring push event on non-promotion branch"
			logger.Info(msg)
			return Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}, nil
		}

		var pr *github.PullRequest
		if pr, _ = pCtx.FindPullRequest(ctx); pr != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion request")
		}

		if pr == nil {
			logger.Info("creating promotion request")
			pr, err = pCtx.CreatePullRequest(ctx)
			if err != nil {
				logger.Error("failed to create promotion request", slog.Any("error", err))
				return Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
			}
			logger.Info("created promotion request", slog.String("url", *pr.URL))
		}

		return Response{
			Body:       fmt.Sprintf("Created promotion request: %s", pr.GetURL()),
			StatusCode: http.StatusCreated,
		}, nil

	case *github.PullRequestEvent:
		logger.Debug("processing pull request event...")
		switch *e.Action {
		case "opened", "edited", "ready_for_review", "reopened", "unlocked":
			// pass
		default:
			logger.Info("ignoring pull request event", slog.String("action", *e.Action))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = promotion.StageRef(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = promotion.StageRef(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request event")

	case *github.PullRequestReviewEvent:
		logger.Debug("processing pull request review event...")
		if *e.Review.State != "approved" {
			logger.Info("ignoring non-approved pull request review event", slog.String("state", *e.Review.State))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = promotion.StageRef(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = promotion.StageRef(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request review event")

	case *github.CheckSuiteEvent:
		logger.Debug("processing check suite event...")
		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("ignoring incomplete check suite event", slog.String("status", *e.CheckSuite.Status))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && app.Promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = promotion.StageRef(*pr.Base.Ref)
				pCtx.HeadRef = promotion.StageRef(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion request")
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("parsed check suite event")

	case *github.DeploymentStatusEvent:
		logger.Info("processing deployment status event...")
		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("Ignoring non-success deployment status event", slog.String("state", state))
			return Response{StatusCode: http.StatusFailedDependency}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = promotion.StageRef(*e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		logger.Info("parsed deployment status event")

	case *github.StatusEvent:
		logger.Debug("processing status event...")
		state := *e.State
		if state != "success" {
			logger.Info("Ignoring non-success status event", slog.String("state", state))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.SHA

		logger.Info("parsed status event")

	case *github.WorkflowRunEvent:
		logger.Debug("processing workflow run event...")
		status := *e.WorkflowRun.Status
		if status != "completed" {
			logger.Info("ignoring incomplete workflow run event", slog.String("status", status))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		conclusion := *e.WorkflowRun.Conclusion
		if conclusion != "success" {
			logger.Info("ignoring unsuccessful workflow run event", slog.String("conclusion", conclusion))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.WorkflowRun.HeadSHA

		for _, pr := range e.WorkflowRun.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && app.Promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = promotion.StageRef(*pr.Base.Ref)
				pCtx.HeadRef = promotion.StageRef(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("Ignoring check suite event without matching promotion request")
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("parsed workflow run event")

	default:
		logger.Warn("unhandled event type", slog.String("eventType", eventType))
		return Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	// ignore events without an open promotion request
	if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
		// find matching promotion request by head SHA and populate missing refs
		if _, err = pCtx.FindPullRequest(ctx); err != nil {
			logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	if err = pCtx.FastForwardRefToSha(ctx); err != nil {
		return Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
	}
	logger.Info("fast forward complete")
	return Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}, nil
}
