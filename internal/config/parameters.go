// Package config provides a centralized entrypoint for the application parameters.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

var (
	// Global is a struct that contains the global configuration.
	Global global
	// GitHub is a struct that contains the configuration for GitHub.
	GitHub github
	// Promotion is a struct that contains the configuration for promotion.
	Promotion promotion
	// Service is a struct that contains the configuration for the service mode.
	Service service
	// Lambda is a struct that contains the configuration for the lambda mode.
	Lambda lambda
)

type global struct {
	// Mode is the runtime mode of the application.
	Mode string `yaml:"mode,omitempty" default:"lambda"`
	// Logging is a struct that contains the logging configuration.
	Logging struct {
		// Verbosity is the verbosity level of the application. It represents slog levels.
		Verbosity int `yaml:"verbosity,omitempty"`
		// CallerTrace is a flag that enables the caller trace in the logger.
		CallerTrace bool `yaml:"callerTrace,omitempty"`
	} `yaml:"logging,omitempty"`
	// S3 is a struct that contains the configuration for S3.
	S3 struct {
		Upload struct {
			BucketName string `yaml:"bucketName,omitempty"`
			Enabled    bool   `yaml:"enabled,omitempty"`
		} `yaml:"upload,omitempty"`
	} `yaml:"s3,omitempty"`
}

type promotion struct {
	// Dynamic is a struct that contains the configuration for dynamic promotion.
	Dynamic struct {
		Enabled bool `yaml:"enabled,omitempty" default:"true"`
		// Key is the custom property key to use when fetching the dynamic promoter configuration.
		Key string `yaml:"key,omitempty" default:"gitops-promotion-path"`
		// Class is the custom property class to use when fetching the dynamic promoter configuration.
		Class string `yaml:"class,omitempty" default:"gitops-promotion-class"`
	} `yaml:"dynamicPromotion,omitempty"`
	// DefaultStages is a slice of default promotion stages.
	DefaultStages []string `yaml:"defaultStages,omitempty" default:"[\"main\", \"staging\", \"canary\", \"production\"]"`
	// Events is a slice of GitHub webhook events to listen to.
	Events []string `yaml:"events,omitempty" default:"[\"push\", \"pull_request\", \"pull_request_review\", \"deployment_status\", \"status\", \"check_suite\", \"workflow_run\"]"`
	// Push is a struct that contains the configuration for pushing changes.
	Push struct {
		// CreatePullRequestInDraftModeKey is the key to use to inspect the repository custom properties for draft PR creation.
		CreatePullRequestInDraftModeKey string `yaml:"createPullRequestInDraftModeKey,omitempty" default:"gitops-promotion-draft-pr"`
		// CreateTargetRef is a flag that enables the creation of missing target branches.
		CreateTargetRef bool `yaml:"createTargetRef,omitempty" default:"true"`
	} `yaml:"push,omitempty"`
	// Feedback is a struct that contains the configuration for feedback.
	Feedback struct {
		CommitStatus struct {
			Enabled bool   `yaml:"enabled,omitempty" default:"false"`
			Context string `yaml:"context,omitempty" default:"{source}→{target}"`
		} `yaml:"commitStatus,omitempty"`
		CheckRun struct {
			Enabled bool   `yaml:"enabled,omitempty" default:"true"`
			Name    string `yaml:"name,omitempty" default:"{source}→{target}"`
		} `yaml:"checkRun,omitempty"`
	} `yaml:"feedback,omitempty"`
}

type github struct {
	AuthMode      string `yaml:"authMode,omitempty" default:"ssm"`
	SSMKey        string `yaml:"ssmKey,omitempty"`
	WebhookSecret string `yaml:"webhookSecret,omitempty"`
}

type service struct {
	Path    string        `yaml:"path,omitempty" default:"/"`
	Addr    string        `yaml:"addr,omitempty"`
	Port    string        `yaml:"port,omitempty" default:"8080"`
	Timeout time.Duration `yaml:"timeout,omitempty" default:"5s"`
}

type lambda struct {
	PayloadType string `yaml:"payloadType,omitempty" default:"api-gateway-v2"`
}

// SetDefaults sets the default values for the configuration.
func SetDefaults() error {
	return errors.Join(
		defaults.Set(&Global),
		defaults.Set(&GitHub),
		defaults.Set(&Promotion),
		defaults.Set(&Service),
		defaults.Set(&Lambda),
	)
}

// LoadFromFile loads the configuration from a file.
func LoadFromFile(path string) error {
	if len(path) == 0 {
		return nil
	}
	fstat, err := os.Stat(path)
	if err != nil {
		return nil //nolint:nilerr // If the file does not exist, we ignore it.
	}
	if fstat.IsDir() {
		return fmt.Errorf("configuration file %s is a directory", path)
	}
	if !fstat.Mode().IsRegular() {
		return fmt.Errorf("configuration file %s is not a regular file", path)
	}

	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to read configuration file %s: %w", path, err)
	}
	type all struct {
		Global    global    `yaml:"global,omitempty"`
		Promotion promotion `yaml:"promotion,omitempty"`
		Service   service   `yaml:"service,omitempty"`
		Lambda    lambda    `yaml:"lambda,omitempty"`
		GitHub    github    `yaml:"github,omitempty"`
	}
	var a all
	if err = yaml.Unmarshal(content, &a); err != nil {
		return fmt.Errorf("failed to unmarshal configuration file %s: %w", path, err)
	}
	Global = a.Global
	Promotion = a.Promotion
	Service = a.Service
	Lambda = a.Lambda
	GitHub = a.GitHub

	return nil
}
