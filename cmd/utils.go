package cmd

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var replacer = strings.NewReplacer(".", "_", "-", "_")

type argType interface {
	string | bool | int | time.Duration | []string
}

func bindEnvMap[T argType](cmd *cobra.Command, m map[*T]boundEnvVar[T]) {
	for v, cfg := range m {
		desc := cfg.Description
		var envKey string
		if cfg.Env != nil {
			desc = fmt.Sprintf("[%s] %s", *cfg.Env, desc)
			_ = viper.BindEnv(cfg.Name, *cfg.Env)
			envKey = *cfg.Env
		} else {
			desc = fmt.Sprintf("[%s] %s", strings.ToUpper(replacer.Replace(cfg.Name)), desc)
			_ = viper.BindEnv(cfg.Name, strings.ToUpper(replacer.Replace(cfg.Name)))
			envKey = strings.ToUpper(replacer.Replace(cfg.Name))
		}

		viper.SetDefault(envKey, any(*v))
		switch vt := any(v).(type) {
		case *string:
			vv := viper.GetString(envKey)
			if cfg.Short == nil {
				cmd.PersistentFlags().StringVar(vt, cfg.Name, vv, desc)
			} else {
				cmd.PersistentFlags().StringVarP(vt, cfg.Name, *cfg.Short, vv, desc)
			}
		case *bool:
			vv := viper.GetBool(envKey)
			if cfg.Short == nil {
				cmd.PersistentFlags().BoolVar(vt, cfg.Name, vv, desc)
			} else {
				cmd.PersistentFlags().BoolVarP(vt, cfg.Name, *cfg.Short, vv, desc)
			}
		case *int:
			vv := viper.GetInt(envKey)
			if cfg.Short == nil {
				cmd.PersistentFlags().CountVar(vt, cfg.Name, desc)
			} else {
				cmd.PersistentFlags().CountVarP(vt, cfg.Name, *cfg.Short, desc)
			}
			_ = cmd.PersistentFlags().Lookup(cfg.Name).Value.Set(strconv.Itoa(vv))
		case *time.Duration:
			vv := viper.GetDuration(envKey)
			if cfg.Short == nil {
				cmd.PersistentFlags().DurationVar(vt, cfg.Name, vv, desc)
			} else {
				cmd.PersistentFlags().DurationVarP(vt, cfg.Name, *cfg.Short, vv, desc)
			}
		case *[]string:
			vv := viper.GetStringSlice(envKey)
			if cfg.Short == nil {
				cmd.PersistentFlags().StringSliceVar(vt, cfg.Name, vv, desc)
			} else {
				cmd.PersistentFlags().StringSliceVarP(vt, cfg.Name, *cfg.Short, vv, desc)
			}
		default:
			log.Panicf("command-args parsing error: unhandled default case for type %T", vt)
		}

		_ = viper.BindPFlag(cfg.Name, cmd.PersistentFlags().Lookup(cfg.Name))

		if cfg.Hidden {
			_ = cmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}
}
