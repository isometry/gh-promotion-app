package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log/slog"
	"os"
	"strings"
	"time"
)

func Execute() error {
	return rootCmd.Execute()
}

var (
	serviceMode bool

	dynamicPromoterKey string
	dynamicPromoter    bool

	githubAuthMode string
	githubToken    string
	githubSSMKey   string
	webhookSecret  string

	logger      *slog.Logger
	verbosity   int
	callerTrace bool
)

type boundEnvVar[T string | bool | int | time.Duration] struct {
	Name, Description string
	Env, Short        *string
	Hidden            bool
	Default           *T
}

var rootCmd = &cobra.Command{
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		loadViperVariables(cmd)

		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: callerTrace,
			Level:     slog.LevelWarn - slog.Level(verbosity*4),
		}))
		return nil
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if serviceMode {
			return serviceCmd.PreRunE(cmd, args)
		}
		return lambdaCmd.PreRunE(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if serviceMode {
			return serviceCmd.RunE(cmd, args)
		}
		return lambdaCmd.RunE(cmd, args)
	},
}

func init() {
	viper.AutomaticEnv()
	viper.EnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	bindEnvMap(rootCmd, envMapString)
	bindEnvMap(rootCmd, envMapBool)
	bindEnvMap(rootCmd, envMapCount)

	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(lambdaCmd)
}
