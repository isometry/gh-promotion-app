package cmd

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
)

var (
	testPromoterStages = []string{"test-src", "test-dest"}
	testRepository     = os.Getenv("TEST_SUITE_REPOSITORY")
)

type testCase struct {
	Name                string
	ReceivedRequest     string
	Headers             map[string]string
	ExpectedStatus      int
	EnableWebhookSecret bool
	CreateEmptyCommit   bool
}

func TestHandleEvent(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skipf("GITHUB_TOKEN not set")
	}

	if testRepository == "" {
		t.Skipf("TEST_SUITE_REPOSITORY not set")
	}

	testCases := []testCase{
		{
			Name:           "missing_event_type",
			ExpectedStatus: http.StatusUnprocessableEntity,
		},
		{
			Name:           "invalid_event_type",
			ExpectedStatus: http.StatusBadRequest,
			Headers: map[string]string{
				github.EventTypeHeader: "invalid",
			},
		},
		{
			Name:            "missing_webhook",
			ExpectedStatus:  http.StatusForbidden,
			ReceivedRequest: `{"key": "value"}`,
			Headers: map[string]string{
				"Content-Type":               "application/json",
				github.SHA256SignatureHeader: "sha256=844d7743b13e1bdd66b003c29ebe5184dcf985434dde9f125952595cd533213e",
				github.EventTypeHeader:       "push",
			},
		},
		{
			Name:            "valid_push",
			ExpectedStatus:  http.StatusCreated,
			ReceivedRequest: validPayloadPushEvent,
			Headers: map[string]string{
				"Content-Type":         "application/json",
				github.EventTypeHeader: "push",
			},
			EnableWebhookSecret: true,
			CreateEmptyCommit:   true,
		},
		{
			Name:            "valid_deployment_status_pending",
			ExpectedStatus:  http.StatusFailedDependency,
			ReceivedRequest: validPayloadDeploymentStatusPending,
			Headers: map[string]string{
				"Content-Type":         "application/json",
				github.EventTypeHeader: "deployment_status",
			},
			EnableWebhookSecret: true,
		},
		{
			Name:            "valid_deployment_status_success",
			ExpectedStatus:  http.StatusNoContent,
			ReceivedRequest: validPayloadDeploymentStatusSuccess,
			Headers: map[string]string{
				"Content-Type":         "application/json",
				github.EventTypeHeader: "deployment_status",
			},
			EnableWebhookSecret: true,
		},
	}

	var ref string
	for _, tc := range testCases {
		t.Run(tc.Name, func(tt *testing.T) {
			if tc.CreateEmptyCommit {
				headRef, err := createEmptyCommit(slog.LevelError)
				if err != nil {
					t.Fatalf("failed to createEmptyCommit test: %v", err)
				}
				t.Logf("test suite ref: %s", headRef)
				ref = headRef
				// Artificial delay to allow the empty commit to be created
				<-time.After(3 * time.Second)
			}

			rr := runTest(tt, tc, ref, slog.LevelError)
			// Assertions
			assert.Equal(tt, tc.ExpectedStatus, rr.Code)
			if tt.Failed() {
				tt.Logf("payload: %s", renderPayload(tc.ReceivedRequest, ref, testRepository))
				_ = runTest(tt, tc, ref, slog.LevelDebug)
				tt.FailNow()
			}
		})
	}
}

var dummyWebhookKey = "key"

func runTest(t *testing.T, tc testCase, headRef string, level slog.Leveler) *httptest.ResponseRecorder {
	payload := renderPayload(tc.ReceivedRequest, headRef, testRepository)
	if tc.EnableWebhookSecret {
		_ = os.Setenv("GITHUB_WEBHOOK_SECRET", dummyWebhookKey)
		defer func() {
			_ = os.Unsetenv("GITHUB_WEBHOOK_SECRET")
		}()
		tc.Headers[github.SHA256SignatureHeader] = fmt.Sprintf("sha256=%s", generateHmacSha256(payload, dummyWebhookKey))
	}
	rtm := setupRuntime(t, tc, level)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	for k, v := range tc.Headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	rtm.ServeHTTP(rr, req)

	return rr
}

func setupRuntime(t *testing.T, tc testCase, level slog.Leveler) *runtime.Runtime {
	testLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     level,
	})).With("test", tc.Name)
	hdl, err := handler.NewPromotionHandler(
		handler.WithWebhookSecret(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		handler.WithToken(os.Getenv("GITHUB_TOKEN")),
		handler.WithContext(context.Background()),
		handler.WithAuthMode("token"),
		handler.WithLogger(testLogger.With("component", "promotion-handler")))
	if err != nil {
		t.Fatalf("failed to create promotion handler: %v", err)
	}
	return runtime.NewRuntime(hdl,
		runtime.WithLogger(testLogger.With("component", "runtime")))
}

func createEmptyCommit(level slog.Leveler) (string, error) {
	ref, err := controllers.GitHubEmptyCommitOnBranchWithDefaultClient(
		context.Background(), githubv4.CreateCommitOnBranchInput{
			Branch: githubv4.CommittableBranch{
				RepositoryNameWithOwner: githubv4.NewString(githubv4.String(testRepository)),
				BranchName:              githubv4.NewString(githubv4.String(testPromoterStages[0])),
			},
			Message: githubv4.CommitMessage{
				Body: githubv4.NewString(githubv4.String(fmt.Sprintf("test(auto): empty testing commit [%v]", time.Now().Format(time.RFC3339)))),
			},
			FileChanges: &githubv4.FileChanges{
				Additions: nil,
			},
		},
		controllers.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))),
		controllers.WithAuthMode("token"),
		controllers.WithToken(os.Getenv("GITHUB_TOKEN")))
	if err != nil {
		return "", errors.Wrap(err, "failed to create empty commit")
	}
	return ref, nil
}

func renderPayload(payload string, headRef, fullName string) string {
	parts := strings.Split(testRepository, "/")
	owner := parts[0]
	repository := parts[1]

	tmpl, _ := template.New("payload").Parse(payload)
	var buf strings.Builder
	_ = tmpl.Execute(&buf, struct {
		Ref, Owner, Repository, FullName, Stage string
	}{
		Ref:        headRef,
		Repository: repository,
		Owner:      owner,
		FullName:   fullName,
		Stage:      testPromoterStages[0],
	})
	return buf.String()
}
