package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/helpers"
)

var envMapString = map[*string]boundEnvVar[string]{
	&config.Global.Mode: {
		Name:        "mode",
		Description: "The application runtime mode. Possible values are 'lambda' and 'service'",
		Short:       helpers.Ptr("m"),
	},
	&config.GitHub.AuthMode: {
		Name:        "github-auth-mode",
		Description: "Authentication credentials provider. Supported values are 'token' and 'ssm'.",
		Short:       helpers.Ptr("A"),
	},
	&config.GitHub.SSMKey: {
		Name:        "github-app-ssm-arn",
		Description: "The SSM parameter key to use when fetching GitHub App credentials",
	},
	&config.GitHub.WebhookSecret: {
		Name:        "github-webhook-secret",
		Description: "The secret to use when validating incoming GitHub webhook payloads. If not specified, no validation is performed",
	},
	&config.Promotion.Dynamic.Key: {
		Name:        "promotion-dynamic-custom-property-key",
		Description: "The key to use when fetching the dynamic promoter configuration",
		Env:         helpers.Ptr("DYNAMIC_PROMOTION_KEY"),
	},
	&config.Promotion.Feedback.CommitStatus.Context: {
		Name:        "feedback-commit-status-context",
		Description: "The context key to use when pushing the commit status to the repository. Supported placeholders: {source}, {target}",
	},
	&config.Promotion.Feedback.CheckRun.Name: {
		Name:        "feedback-check-run-name",
		Description: "The name to use when creating the check run. Supported placeholders: {source}, {target}",
	},
	&config.Global.S3.Upload.BucketName: {
		Name:        "promotion-report-s3-upload-bucket",
		Description: "The S3 bucket to use when uploading promotion reports",
		Env:         helpers.Ptr("PROMOTION_REPORT_S3_BUCKET"),
	},
}

var envMapBool = map[*bool]boundEnvVar[bool]{
	&config.Global.Logging.CallerTrace: {
		Name:        "verbosity-caller-trace",
		Description: "Enable caller trace in logs",
		Short:       helpers.Ptr("V"),
	},
	&config.Promotion.Dynamic.Enabled: {
		Name:        "promotion-dynamic",
		Description: "Enable dynamic promotion",
		Env:         helpers.Ptr("PROMOTION_DYNAMIC"),
	},
	&config.Promotion.Push.CreateTargetRef: {
		Name:        "create-missing-target-branches",
		Description: "Create missing target branches",
	},
	&config.Promotion.Feedback.CommitStatus.Enabled: {
		Name:        "feedback-commit-status",
		Description: "Enable commit status feedback",
	},
	&config.Promotion.Feedback.CheckRun.Enabled: {
		Name:        "feedback-check-run",
		Description: "Enable check-run feedback",
	},
	&config.Global.S3.Upload.Enabled: {
		Name:        "promotion-report-s3-upload",
		Description: "Enable S3 upload of promotion reports",
		Env:         helpers.Ptr("PROMOTION_REPORT_S3_UPLOAD"),
	},
}

var envMapCount = map[*int]boundEnvVar[int]{
	&config.Global.Logging.Verbosity: {
		Name:        "verbosity",
		Description: "Increase logger verbosity (default WarnLevel)",
		Short:       helpers.Ptr("v"),
	},
}

var envMapStringSlice = map[*[]string]boundEnvVar[[]string]{
	&config.Promotion.DefaultStages: {
		Name:        "promotion-default-stages",
		Description: "The default promotion stages",
	},
	&config.Promotion.Events: {
		Name:        "promotion-events",
		Description: "The GitHub promotion events to listen to",
	},
}
