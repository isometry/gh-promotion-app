package cmd

import (
	"fmt"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
	"time"
)

type argType interface {
	string | bool | int | time.Duration
}

var envMapString = map[*string]boundEnvVar[string]{
	&githubToken: {
		Name:        "github-token",
		Description: "When specified, the GitHub token to use for API requests",
		Hidden:      true,
	},
	&githubAuthMode: {
		Name:        "github-auth-mode",
		Description: "Authentication serviceMode. Supported values are 'token' and 'ssm'. If token is specified, and the GITHUB_TOKEN environment variable is not set, the 'ssm' serviceMode is used as an automatic fallback.",
		Short:       helpers.Ptr("A"),
	},
	&githubSSMKey: {
		Name:        "github-app-ssm-arn",
		Description: "The SSM parameter key to use when fetching GitHub App credentials",
	},
	&webhookSecret: {
		Name:        "github-webhook-secret",
		Description: "The secret to use when validating incoming GitHub webhook payloads. If not specified, no validation is performed",
	},
	&dynamicPromoterKey: {
		Name:        "promotion-custom-attribute",
		Description: "The key to use when fetching the dynamic promoter configuration",
		Default:     helpers.Ptr("gitops-promotion-path"),
	},
}

var envMapBool = map[*bool]boundEnvVar[bool]{
	&serviceMode: {
		Name:        "service",
		Description: "If set to true, the service will run in 'service' mode. Otherwise, it will run in 'lambda' mode by default",
	},
	&callerTrace: {
		Name:        "caller-trace",
		Description: "Enable caller trace in logs",
		Short:       helpers.Ptr("V"),
	},
	&dynamicPromoter: {
		Name:        "promotion-dynamic",
		Description: "Enable dynamic promotion",
	},
}

var envMapCount = map[*int]boundEnvVar[int]{
	&verbosity: {
		Name:        "verbose",
		Description: "Increase logger verbosity (default WarnLevel)",
		Short:       helpers.Ptr("v"),
	},
}

func bindEnvMap[T argType](cmd *cobra.Command, m map[*T]boundEnvVar[T]) {

	for v, cfg := range m {
		desc := cfg.Description
		if cfg.Env != nil {
			desc = fmt.Sprintf("[%s] %s", *cfg.Env, desc)
		} else {
			desc = fmt.Sprintf("[%s] %s", strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(cfg.Name)), desc)
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
			_ = viper.BindEnv(cfg.Name, strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(cfg.Name)))
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
