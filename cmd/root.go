// Package cmd provides the entrypoint for the gh-promotion-app.
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Execute runs the root command, handling command-line arguments and invoking the corresponding functionality.
func Execute() error {
	return rootCmd.Execute()
}

var (
	runtimeMode    string
	githubAuthMode string
	githubToken    string
	githubSSMKey   string
	webhookSecret  string

	logger      *slog.Logger
	verbosity   int
	callerTrace bool
)

type boundEnvVar[T argType] struct {
	Name, Description string
	Env, Short        *string
	Hidden            bool
	Default           *T
}

var rootCmd = &cobra.Command{
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		loadViperVariables(cmd)

		runtimeMode = strings.TrimSpace(runtimeMode)
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: callerTrace,
			Level:     slog.LevelWarn - slog.Level(verbosity*4),
		}))
		return nil
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		switch runtimeMode {
		case "service":
			return serviceCmd.PreRunE(cmd, args)
		case "lambda":
			return lambdaCmd.PreRunE(cmd, args)
		default:
			return fmt.Errorf("invalid mode: %s", runtimeMode)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch runtimeMode {
		case "service":
			return serviceCmd.RunE(cmd, args)
		case "lambda":
			return lambdaCmd.RunE(cmd, args)
		default:
			return fmt.Errorf("invalid mode: %s", runtimeMode)
		}
	},
}

func init() {
	viper.AutomaticEnv()
	viper.EnvKeyReplacer(replacer)

	bindEnvMap(rootCmd, envMapString)
	bindEnvMap(rootCmd, envMapBool)
	bindEnvMap(rootCmd, envMapCount)

	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(lambdaCmd)
}
