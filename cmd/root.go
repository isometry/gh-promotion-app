package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log/slog"
	"os"
)

func Execute() error {
	return rootCmd.Execute()
}

var (
	githubAuthMode string
	githubToken    string
	githubSSMKey   string
	webhookSecret  string
	logger         *slog.Logger
	verbosity      int
	callerTrace    bool
)

type boundEnvVar struct {
	Name, Env, Description string
	Short                  *string
	Hidden                 bool
}

var envMap = map[*string]boundEnvVar{
	&githubToken: {
		Name:        "github.token",
		Env:         "GITHUB_TOKEN",
		Description: "When specified, the GitHub token to use for API requests",
		Hidden:      true,
	},
	&githubAuthMode: {
		Name:        "github.auth-mode",
		Env:         "GITHUB_AUTH_MODE",
		Description: "Authentication mode. Supported values are 'token' and 'ssm'. If token is specified, and the GITHUB_TOKEN environment variable is not set, the 'ssm' mode is used as an automatic fallback.",
		Short:       helpers.StringPtr("A"),
	},
	&githubSSMKey: {
		Name:        "github.ssm-key",
		Env:         "GITHUB_APP_SSM_ARN",
		Description: "The SSM parameter key to use when fetching GitHub App credentials",
	},
	&webhookSecret: {
		Name: "github.webhook-secret",
		Env:  "GITHUB_WEBHOOK_SECRET",
	},
}

var rootCmd = &cobra.Command{
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: callerTrace,
			Level:     slog.LevelWarn - slog.Level(verbosity*4),
		}))
		return nil
	},
}

func init() {
	bindEnvMap(envMap)

	rootCmd.PersistentFlags().CountVarP(&verbosity, "logger.verbose", "v", "increase logger verbosity")
	rootCmd.PersistentFlags().BoolVarP(&callerTrace, "logger.caller-trace", "V", false, "enable logger caller trace")
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(lambdaCmd)
}

func bindEnvMap(m map[*string]boundEnvVar) {
	viper.AutomaticEnv()
	for v, cfg := range m {
		_ = viper.BindEnv(cfg.Env)
		if cfg.Short == nil {
			rootCmd.PersistentFlags().StringVar(v, cfg.Name, viper.GetString(cfg.Env), cfg.Description)
		} else {
			rootCmd.PersistentFlags().StringVarP(v, cfg.Name, *cfg.Short, viper.GetString(cfg.Env), cfg.Description)
		}
		if cfg.Hidden {
			_ = rootCmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}

}
