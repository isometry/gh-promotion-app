package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type argType interface {
	string | bool | int | time.Duration
}

var envMapString = map[*string]boundEnvVar[string]{
	&runtimeMode: {
		Name:        "mode",
		Description: "The application runtime mode. Possible values are 'lambda' and 'service'",
		Default:     helpers.Ptr("lambda"),
	},
	&lambdaPayloadType: {
		Name:        "lambda-payload-type",
		Description: "The payload type to expect when running in Lambda mode. Supported values are 'api-gateway-v1', 'api-gateway-v2' and 'lambda-url'",
		Default:     helpers.Ptr("api-gateway-v2"),
		Env:         helpers.Ptr("LAMBDA_PAYLOAD_TYPE"),
	},
	&githubToken: {
		Name:        "github-token",
		Description: "When specified, the GitHub token to use for API requests",
		Hidden:      true,
	},
	&githubAuthMode: {
		Name:        "github-auth-mode",
		Description: "Authentication credentials provider. Supported values are 'token' and 'ssm'.",
		Short:       helpers.Ptr("A"),
		Default:     helpers.Ptr("ssm"),
	},
	&githubSSMKey: {
		Name:        "github-app-ssm-arn",
		Description: "The SSM parameter key to use when fetching GitHub App credentials",
	},
	&webhookSecret: {
		Name:        "github-webhook-secret",
		Description: "The secret to use when validating incoming GitHub webhook payloads. If not specified, no validation is performed",
	},
	&capabilities.Promotion.DynamicPromotion.Key: {
		Name:        "promotion-dynamic-custom-property-key",
		Description: "The key to use when fetching the dynamic promoter configuration",
		Env:         helpers.Ptr("DYNAMIC_PROMOTION_KEY"),
		Default:     helpers.Ptr("gitops-promotion-path"),
	},
	&capabilities.Promotion.Feedback.Context: {
		Name:        "feedback-commit-status-context",
		Description: "The context key to use when pushing the commit status to the repository. Supported placeholders: {source}, {target}",
		Env:         helpers.Ptr("FEEDBACK_COMMIT_STATUS_CONTEXT"),
		Default:     helpers.Ptr("{source}→{target}"),
	},
	&capabilities.Global.S3.Upload.BucketName: {
		Name:        "promotion-report-s3-upload-bucket",
		Description: "The S3 bucket to use when uploading promotion reports",
		Env:         helpers.Ptr("PROMOTION_REPORT_S3_BUCKET"),
		Default:     helpers.Ptr(os.Getenv("S3_BUCKET_NAME")),
	},
}

var envMapBool = map[*bool]boundEnvVar[bool]{
	&callerTrace: {
		Name:        "caller-trace",
		Description: "Enable caller trace in logs",
		Short:       helpers.Ptr("V"),
	},
	&capabilities.Promotion.DynamicPromotion.Enabled: {
		Name:        "promotion-dynamic",
		Description: "Enable dynamic promotion",
		Env:         helpers.Ptr("DYNAMIC_PROMOTION"),
	},
	&capabilities.Promotion.Push.CreateTargetRef: {
		Name:        "create-missing-target-branches",
		Description: "Create missing target branches",
		Env:         helpers.Ptr("CREATE_MISSING_TARGET_BRANCHES"),
		Default:     helpers.Ptr(true),
	},
	&capabilities.Promotion.Feedback.Enabled: {
		Name:        "feedback-commit-status",
		Description: "Enable feedback commit status",
		Env:         helpers.Ptr("FEEDBACK_COMMIT_STATUS"),
		Default:     helpers.Ptr(true),
	},
	&capabilities.Global.FetchRateLimits: {
		Name:        "fetch-rate-limits",
		Description: "Enable per-event fetching of rate limits and corresponding logs decoration",
		Env:         helpers.Ptr("FETCH_RATE_LIMITS"),
		Default:     helpers.Ptr(true),
	},
	&capabilities.Global.S3.Upload.Enabled: {
		Name:        "promotion-report-s3-upload",
		Description: "Enable S3 upload of promotion reports",
		Env:         helpers.Ptr("PROMOTION_REPORT_S3_UPLOAD"),
	},
}

var envMapCount = map[*int]boundEnvVar[int]{
	&verbosity: {
		Name:        "verbose",
		Description: "Increase logger verbosity (default WarnLevel)",
		Short:       helpers.Ptr("v"),
	},
}

var replacer = strings.NewReplacer(".", "_", "-", "_")

func bindEnvMap[T argType](cmd *cobra.Command, m map[*T]boundEnvVar[T]) {
	for v, cfg := range m {
		desc := cfg.Description
		if cfg.Env != nil {
			desc = fmt.Sprintf("[%s] %s", *cfg.Env, desc)
		} else {
			desc = fmt.Sprintf("[%s] %s", strings.ToUpper(replacer.Replace(cfg.Name)), desc)
		}

		switch any(v).(type) {
		case *string:
			var def string
			if cfg.Env != nil {
				def = viper.GetString(*cfg.Env)
			}
			if cfg.Default != nil {
				def = (any(*cfg.Default)).(string)
			}
			sv := any(v).(*string)
			if cfg.Short == nil {
				cmd.PersistentFlags().StringVar(sv, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().StringVarP(sv, cfg.Name, *cfg.Short, def, desc)
			}
		case *bool:
			var def bool
			if cfg.Env != nil {
				def = viper.GetBool(*cfg.Env)
			}
			if cfg.Default != nil {
				def = (any(*cfg.Default)).(bool)
			}
			bv := any(v).(*bool)
			if cfg.Short == nil {
				cmd.PersistentFlags().BoolVar(bv, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().BoolVarP(bv, cfg.Name, *cfg.Short, def, desc)
			}
		case *int:
			iv := any(v).(*int)
			if cfg.Short == nil {
				cmd.PersistentFlags().CountVar(iv, cfg.Name, desc)
			} else {
				cmd.PersistentFlags().CountVarP(iv, cfg.Name, *cfg.Short, desc)
			}
		case *time.Duration:
			var def time.Duration
			if cfg.Env != nil {
				def = viper.GetDuration(*cfg.Env)
			}
			if cfg.Default != nil {
				def = (any(*cfg.Default)).(time.Duration)
			}
			dv := any(v).(*time.Duration)
			if cfg.Short == nil {
				cmd.PersistentFlags().DurationVar(dv, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().DurationVarP(dv, cfg.Name, *cfg.Short, def, desc)
			}
		default:
			panic("unhandled default case")
		}

		if cfg.Env != nil {
			_ = viper.BindEnv(cfg.Name, *cfg.Env)
		} else {
			_ = viper.BindEnv(cfg.Name, strings.ToUpper(replacer.Replace(cfg.Name)))
		}

		if cfg.Hidden {
			_ = cmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}
}

func loadViperVariables(cmd *cobra.Command) {
	for _, key := range viper.AllKeys() {
		f := cmd.Flags().Lookup(key)
		if f != nil && viper.Get(key) != nil {
			_ = cmd.Flags().Lookup(key).Value.Set(viper.GetString(key))
		}
	}
}
