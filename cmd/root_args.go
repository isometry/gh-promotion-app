package cmd

import (
	"fmt"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

type argType interface {
	string | bool | int | time.Duration
}

var envMapString = map[*string]boundEnvVar[string]{
	&githubToken: {
		Name:        "github.token",
		Env:         "GITHUB_TOKEN",
		Description: "When specified, the GitHub token to use for API requests",
		Hidden:      true,
	},
	&githubAuthMode: {
		Name:        "github.auth-serviceMode",
		Env:         "GITHUB_AUTH_MODE",
		Description: "Authentication serviceMode. Supported values are 'token' and 'ssm'. If token is specified, and the GITHUB_TOKEN environment variable is not set, the 'ssm' serviceMode is used as an automatic fallback.",
		Short:       helpers.Ptr("A"),
	},
	&githubSSMKey: {
		Name:        "github.ssm-key",
		Env:         "GITHUB_APP_SSM_ARN",
		Description: "The SSM parameter key to use when fetching GitHub App credentials",
	},
	&webhookSecret: {
		Name:        "github.webhook-secret",
		Env:         "GITHUB_WEBHOOK_SECRET",
		Description: "The secret to use when validating incoming GitHub webhook payloads. If not specified, no validation is performed",
	},
	&dynamicPromoterKey: {
		Name:        "promotion.dynamic-key",
		Env:         "PROMOTION_DYNAMIC_KEY",
		Description: "The key to use when fetching the dynamic promoter configuration",
		Default:     helpers.Ptr("gitops-promotion-path"),
	},
}

var envMapBool = map[*bool]boundEnvVar[bool]{
	&serviceMode: {
		Name:        "service",
		Env:         "SERVICE_MODE",
		Description: "If set to true, the service will run in 'service' mode. Otherwise, it will run in 'lambda' mode by default",
	},
	&callerTrace: {
		Name:        "logger.caller-trace",
		Env:         "CALLER_TRACE",
		Description: "Enable caller trace in logs",
		Short:       helpers.Ptr("V"),
	},
	&dynamicPromoter: {
		Name:        "promotion.dynamic",
		Env:         "PROMOTION_DYNAMIC",
		Description: "Enable dynamic promotion",
	},
}

var envMapCount = map[*int]boundEnvVar[int]{
	&verbosity: {
		Name:        "logger.verbose",
		Env:         "VERBOSITY",
		Description: "Increase logger verbosity (default WarnLevel)",
		Short:       helpers.Ptr("v"),
	},
}

func bindEnvMap[T argType](cmd *cobra.Command, m map[*T]boundEnvVar[T]) {
	viper.AutomaticEnv()
	for v, cfg := range m {
		_ = viper.BindEnv(cfg.Env)
		desc := fmt.Sprintf("[%s] %s", cfg.Env, cfg.Description)
		switch any(v).(type) {
		case *string:
			def := viper.GetString(cfg.Env)
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
			def := viper.GetBool(cfg.Env)
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
			def := viper.GetDuration(cfg.Env)
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

		if cfg.Hidden {
			_ = cmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}
}
