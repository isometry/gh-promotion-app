package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AWS *struct {
		S3 *struct {
			Bucket *string
		}
		SSM *struct {
			Key *string
		}
	}
	GitHub *struct {
		Token         *string
		AppID         *string `mapstructure:"app_id"`
		AppPrivateKey *string `mapstructure:"app_private_key"`
		WebhookSecret *string `mapstructure:"webhook_secret"`
	}
	DefaultPromotionStages []string
}

func (c *Config) Load() {
	viper.AutomaticEnv() // read in environment variables that match bound variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	viper.BindEnv("aws.s3.bucket", "S3_BUCKET_NAME")

	viper.BindEnv("promotion.stages", "PROMOTION_STAGES")
	viper.SetDefault("promotion.stages", []string{"main", "staging", "canary", "production"})

	viper.SetDefault("aws.ssm.parameter", "gh-promotion-app-creds")

	viper.BindEnv("github.token")
	viper.BindEnv("github.app_id")
	viper.BindEnv("github.app_private_key")
	viper.BindEnv("github.webhook_secret")

	viper.Unmarshal(c)
}
