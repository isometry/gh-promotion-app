package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log/slog"
	"os"
)

func Execute() error {
	return rootCmd.Execute()
}

var (
	githubToken string
	logger      *slog.Logger
	verbosity   int
	callerTrace bool
)
var envMap = map[*string]struct {
	Name, Env, Description string
	Hidden                 bool
}{
	&githubToken: {
		Name:        "github.token",
		Env:         "GITHUB_TOKEN",
		Description: "When specified, the GitHub token to use for API requests",
		Hidden:      true,
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
	viper.AutomaticEnv()
	for v, cfg := range envMap {
		_ = viper.BindEnv(cfg.Env)
		rootCmd.PersistentFlags().StringVar(v, cfg.Name, viper.GetString(cfg.Env), cfg.Description)
		if cfg.Hidden {
			_ = rootCmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}

	rootCmd.PersistentFlags().CountVarP(&verbosity, "logger.verbose", "v", "increase verbosity")
	rootCmd.PersistentFlags().BoolVarP(&callerTrace, "logger.caller-trace", "V", false, "enable logger caller trace")
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(lambdaCmd)
}
