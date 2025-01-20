package cmd

import (
	"fmt"
	"log"
	"os"
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
		if cfg.Env != nil {
			desc = fmt.Sprintf("[%s] %s", *cfg.Env, desc)
		} else {
			desc = fmt.Sprintf("[%s] %s", strings.ToUpper(replacer.Replace(cfg.Name)), desc)
		}

		switch vt := any(v).(type) {
		case *string:
			def := any(*v).(string)
			if cfg.Env != nil {
				vf, found := os.LookupEnv(*cfg.Env)
				if found {
					def = vf
				}
			}
			if cfg.Short == nil {
				cmd.PersistentFlags().StringVar(vt, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().StringVarP(vt, cfg.Name, *cfg.Short, def, desc)
			}
		case *bool:
			def := any(*v).(bool)
			if cfg.Env != nil {
				_, found := os.LookupEnv(*cfg.Env)
				if found {
					def = viper.GetBool(*cfg.Env)
				}
			}
			if cfg.Short == nil {
				cmd.PersistentFlags().BoolVar(vt, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().BoolVarP(vt, cfg.Name, *cfg.Short, def, desc)
			}
		case *int:
			def := any(*v).(int)
			if cfg.Short == nil {
				cmd.PersistentFlags().CountVar(vt, cfg.Name, desc)
			} else {
				cmd.PersistentFlags().CountVarP(vt, cfg.Name, *cfg.Short, desc)
			}
			_ = cmd.PersistentFlags().Lookup(cfg.Name).Value.Set(strconv.Itoa(def))
		case *time.Duration:
			def := any(*v).(time.Duration)
			if cfg.Env != nil {
				_, found := os.LookupEnv(*cfg.Env)
				if found {
					def = viper.GetDuration(*cfg.Env)
				}
			}

			if cfg.Short == nil {
				cmd.PersistentFlags().DurationVar(vt, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().DurationVarP(vt, cfg.Name, *cfg.Short, def, desc)
			}
		case *[]string:
			def := any(*v).([]string)
			if cfg.Env != nil {
				_, found := os.LookupEnv(*cfg.Env)
				if found {
					def = viper.GetStringSlice(*cfg.Env)
				}
			}
			if cfg.Short == nil {
				cmd.PersistentFlags().StringSliceVar(vt, cfg.Name, def, desc)
			} else {
				cmd.PersistentFlags().StringSliceVarP(vt, cfg.Name, *cfg.Short, def, desc)
			}
		default:
			log.Panicf("command-args parsing error: unhandled default case for type %T", vt)
		}

		_ = viper.BindPFlag(cfg.Name, cmd.PersistentFlags().Lookup(cfg.Name))
		if cfg.Env != nil {
			_ = viper.BindEnv(cfg.Name, *cfg.Env)
		} else {
			_ = viper.BindEnv(cfg.Name, strings.ToUpper(replacer.Replace(cfg.Name)))
		}

		if cfg.Hidden {
			_ = cmd.PersistentFlags().MarkHidden(cfg.Name)
		}
	}
}
