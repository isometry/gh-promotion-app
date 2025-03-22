// Package cmd provides the entrypoint for the gh-promotion-app cli.
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

var (
	configFilePath string
	logger         *slog.Logger
)

type boundEnvVar[T argType] struct {
	Name, Description string
	Env, Short        *string
	Hidden            bool
}

// New returns the root command for the gh-promotion-app.
func New() *cobra.Command {
	cmd := &cobra.Command{
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			config.Global.Mode = strings.TrimSpace(config.Global.Mode)
			logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				AddSource: config.Global.Logging.CallerTrace,
				Level:     slog.LevelWarn - slog.Level(config.Global.Logging.Verbosity*4),
			})).With("mode", config.Global.Mode)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch config.Global.Mode {
			case config.ModeService:
				cmd.SetArgs([]string{"service"})
			case config.ModeLambdaHTTP:
				cmd.SetArgs([]string{"lambda", "http"})
			case config.ModeLambdaEvent:
				cmd.SetArgs([]string{"lambda", "event"})
			default:
				return fmt.Errorf("invalid mode: %s", config.Global.Mode)
			}
			return cmd.Execute()
		},
	}

	// Root command flags
	cmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "config.yaml", "path to the configuration file")

	// Configuration loading & defaults
	if err := errors.Join(
		config.LoadFromFile(configFilePath),
		config.SetDefaults(),
	); err != nil {
		panic(err)
	}

	// Dynamic flags
	setupDynamicFlags(cmd)

	// Subcommands
	cmd.AddCommand(
		cmdLambda(),
		cmdService(),
	)

	return cmd
}

func setupDynamicFlags(cmd *cobra.Command) {
	viper.AutomaticEnv()
	viper.EnvKeyReplacer(replacer)

	bindEnvMap(cmd, envMapString)
	bindEnvMap(cmd, envMapBool)
	bindEnvMap(cmd, envMapCount)
	bindEnvMap(cmd, envMapStringSlice)
}
