package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/google/go-github/v59/github"

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
	s3Client   *s3.Client
	ghAppCreds *ghapp.GHAppCredentials
	hmacSecret []byte
	ghClient   *github.Client
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

// type LoggerWithResponder struct{ *slog.Logger }

// func (logger *LoggerWithResponder) InfoResponse(message string, statusCode int) (Response, error) {
// 	logger.Info(message, slog.Int("statusCode", statusCode))
// 	return Response{Body: message, StatusCode: statusCode}, nil
// }

// func (logger *LoggerWithResponder) ErrorResponse(message string, err error) (Response, error) {
// 	logger.Error(message, slog.Any("error", err))
// 	var statusCode int
// 	switch err.(type) {
// 	case *NoCredentialsError:
// 		statusCode = http.StatusInternalServerError
// 	case *NoInstallationIdError:
// 		statusCode = http.StatusUnprocessableEntity
// 	case *NoEventTypeError:
// 		statusCode = http.StatusBadRequest
// 	default:
// 		statusCode = http.StatusUnprocessableEntity
// 	}
// 	return Response{Body: message, StatusCode: statusCode}, nil
// }

func (app *App) RetrieveGhAppCreds(ctx context.Context) error {
	app.Logger.Info("retrieving GitHub App credentials")

	switch {
	case app.AwsConfig != nil:
		return app.RetrieveGhAppCredsFromSSM(ctx)
	}
	return fmt.Errorf("no GitHub App credentials available")
}

func (app *App) RetrieveGhAppCredsFromSSM(ctx context.Context) error {
	app.Logger.Info("Retrieving GitHub App credentials from SSM")

	ssm_client := ssm.NewFromConfig(*app.AwsConfig)

	ssmResponse, err := ssm_client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(os.Getenv("GITHUB_APP_SSM_ARN")),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		app.Logger.Warn("failed to load SSM parameters", slog.Any("error", err))
		return err
	}

	ghaParams := []byte(*ssmResponse.Parameter.Value)

	var ghAppCreds ghapp.GHAppCredentials
	if err = json.Unmarshal(ghaParams, &ghAppCreds); err != nil {
		app.Logger.Warn("failed to unmarshal SSM parameters", slog.Any("error", err))
		return err
	}

	app.ghAppCreds = &ghAppCreds

	return nil
}

func (app *App) RetrieveGhAppCredsFromVault(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (app *App) HandleEvent(ctx context.Context, request Request) (response Response, err error) {
	logger := app.Logger

	logger.Info("handling request")

	eventType := request.Headers[strings.ToLower(github.EventTypeHeader)]
	if eventType == "" {
		logger.Warn("missing event type")
		return Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity}, nil
	}

	pCtx := promotion.Context{
		EventType: &eventType,
	}

	logger = logger.With(slog.Any("context", pCtx))

	if slices.Index(HandledEventTypes, eventType) == -1 {
		logger.Info("unhandled event")
		return Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	// retrieve GitHub App credentials if they are not already set
	if app.ghAppCreds == nil {
		err = app.RetrieveGhAppCreds(ctx)
		if err != nil {
			logger.Error("retrieving credentials", slog.Any("error", err))
			return Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	err = app.ghAppCreds.WebhookSecret.ValidateSignature([]byte(request.Body), request.Headers)
	if err != nil {
		logger.Error("validating signature", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusUnauthorized}, nil
	}
	logger.Debug("request is valid")

	event, err := github.ParseWebHook(eventType, []byte(request.Body))
	if err != nil {
		logger.Error("parsing webhook payload", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity}, nil
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

	var EventInstallationId EventInstallationId
	err = json.Unmarshal([]byte(request.Body), &EventInstallationId)
	if err != nil {
		logger.Warn("no installation ID found", slog.Any("error", err))
		return Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}
	installationId := EventInstallationId.Installation.ID

	logger = logger.With(slog.Int64("installationId", *installationId))

	// XXX: could/should we cache the installation client? …token TTL is only 1 hour
	client, err := app.ghAppCreds.GetInstallationClient(*installationId)
	if err != nil {
		logger.Error("failed to get installation client", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
	}
	pCtx.Client = client

	switch e := event.(type) {
	case *github.PushEvent:
		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = e.Ref // already fully qualified
		pCtx.HeadSHA = e.After

		if stageIndex := promotion.StageIndex(*e.Ref); stageIndex != -1 && stageIndex < len(promotion.Stages)-1 {
			pCtx.BaseRef = promotion.StageRef(promotion.Stages[stageIndex+1])
		} else {
			msg := "ignoring push event on non-promotion branch"
			logger.Info(msg)
			return Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}, nil
		}

		if requestUrl, _ := pCtx.FindRequest(); requestUrl != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion request")
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("creating promotion request")
		pr, err := pCtx.CreateRequest()
		if err != nil {
			logger.Error("failed to create promotion request", slog.Any("error", err))
			return Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
		}
		logger.Info("created promotion request", slog.String("url", *pr.URL))
		return Response{
			Body:       fmt.Sprintf("Created promotion request: %s", pr.GetURL()),
			StatusCode: http.StatusCreated,
		}, nil

	case *github.PullRequestEvent:
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
		if *e.Review.State != "approved" {
			logger.Info("Ignoring non-approved pull request review event", slog.String("state", *e.Review.State))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = promotion.StageRef(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = promotion.StageRef(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request review event")

	case *github.CheckSuiteEvent:
		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("Ignoring incomplete check suite event", slog.String("status", *e.CheckSuite.Status))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && promotion.IsPromotionRequest(pr) {
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
		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("Ignoring non-success deployment status event", slog.String("state", state))
			return Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = promotion.StageRef(*e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		logger.Info("parsed deployment status event")

	case *github.StatusEvent:
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
			if *pr.Head.SHA == *pCtx.HeadSHA && promotion.IsPromotionRequest(pr) {
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
		if _, err = pCtx.FindRequest(); err != nil {
			logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	logger.Info("attempting fast forward")
	if err = pCtx.FastForwardRefToSha(ctx); err != nil {
		logger.Error("Failed to fast forward", slog.Any("error", err))
		return Response{Body: err.Error(), StatusCode: http.StatusFailedDependency}, nil
	}
	logger.Info("fast forward complete")
	return Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}, nil
}
