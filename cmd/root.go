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
	Env, Description string
}{
	&githubToken: {
		Env:         "GITHUB_TOKEN",
		Description: "When specified, the GitHub token to use for API requests",
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
		rootCmd.PersistentFlags().StringVar(v, cfg.Description, viper.GetString(cfg.Env), cfg.Description)
	}

	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "increase verbosity")
	rootCmd.PersistentFlags().BoolVar(&callerTrace, "caller-trace", false, "enable caller trace")
	rootCmd.AddCommand(serviceCmd)
}
