// Package cmd provides the entrypoint for the gh-promotion-app.
package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Execute runs the root command, handling command-line arguments and invoking the corresponding functionality.
func Execute() error {
	return rootCmd.Execute()
}

var (
	configFilePath string
	logger         *slog.Logger
)

type boundEnvVar[T argType] struct {
	Name, Description string
	Env, Short        *string
	Hidden            bool
}

var rootCmd = &cobra.Command{
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		config.Global.Mode = strings.TrimSpace(config.Global.Mode)
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: config.Global.Logging.CallerTrace,
			Level:     slog.LevelWarn - slog.Level(config.Global.Logging.Verbosity*4),
		}))
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch config.Global.Mode {
		case config.ModeService:
			return serviceCmd.RunE(cmd, args)
		case config.ModeLambdaHTTP:
			return lambdaHTTPCmd.RunE(cmd, args)
		case config.ModeLambdaEvent:
			return lambdaEventCmd.RunE(cmd, args)
		default:
			return fmt.Errorf("invalid mode: %s", config.Global.Mode)
		}
	},
}

func init() {
	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "config.yaml", "path to the configuration file")

	// Configuration loading & defaults
	if err := errors.Join(
		config.LoadFromFile(configFilePath),
		config.SetDefaults(),
	); err != nil {
		panic(err)
	}

	// Dynamic flags
	setupDynamicFlags()

	// Subcommands
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(lambdaCmd)
}

func setupDynamicFlags() {
	viper.AutomaticEnv()
	viper.EnvKeyReplacer(replacer)

	bindEnvMap(rootCmd, envMapString)
	bindEnvMap(rootCmd, envMapBool)
	bindEnvMap(rootCmd, envMapCount)
	bindEnvMap(rootCmd, envMapStringSlice)
}
