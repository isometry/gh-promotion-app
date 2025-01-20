// Package aws provides the Controller struct that wraps AWS services and provides S3 and SSM functionality with context and logging support.
package aws

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/logging"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/pkg/errors"
)

// Controller represents a wrapper for Controller services providing S3 and SSM functionality with context and logging support.
type Controller struct {
	ctx    context.Context
	logger *slog.Logger

	config    *aws.Config
	s3Client  *s3.Client
	ssmClient *ssm.Client
}

// Option defines a function type used to configure an instance of the Controller struct.
type Option func(*Controller)

// NewController initializes a Controller with customizable options and default configurations if unspecified.
// It returns an instance of the Controller struct and an error if any required initialization steps fail.
func NewController(opts ...Option) (*Controller, error) {
	_inst := &Controller{}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.logger == nil {
		_inst.logger = helpers.NewNoopLogger()
	}
	_inst.logger = _inst.logger.With("controller", "Controller")
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.config == nil {
		_inst.logger.Debug("loading default Controller configuration...")
		cfg, err := config.LoadDefaultConfig(_inst.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Controller configuration")
		}
		cfg.Logger = newAWSLogger(_inst.logger)
		_inst.config = &cfg
	}

	_inst.s3Client = s3.NewFromConfig(*_inst.config)
	_inst.ssmClient = ssm.NewFromConfig(*_inst.config)
	return _inst, nil
}

// GetSecret retrieves a secret value from Controller SSM Parameter Store using the provided key.
// If encrypted is true, the secret is returned decrypted.
// Returns the secret value as a string pointer or an error if retrieval fails.
func (a *Controller) GetSecret(key string, encrypted bool) (*string, error) {
	a.logger.With("key", key).Debug("fetching SSM secret...")
	ssmResponse, err := a.ssmClient.GetParameter(a.ctx, &ssm.GetParameterInput{
		Name:           aws.String(key),
		WithDecryption: aws.Bool(encrypted),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to load SSM parameters")
	}
	return ssmResponse.Parameter.Value, nil
}

// PutS3Object uploads a JSON object to the specified S3 bucket with a key formatted as a timestamp and the provided ID.
// The method takes the event type, bucket name, and the object body as a byte slice as parameters.
// Returns an error if the S3 upload fails or if the bucket name is empty.
func (a *Controller) PutS3Object(id string, bucket string, body []byte) error {
	if bucket != "" {
		key := fmt.Sprintf("%s.%s", time.Now().UTC().Format(time.RFC3339Nano), id)
		_, err := a.s3Client.PutObject(a.ctx, &s3.PutObjectInput{
			Bucket:      &bucket,
			Key:         aws.String(key),
			Body:        bytes.NewReader(body),
			ContentType: aws.String("application/json"),
		})
		if err != nil {
			return errors.Wrap(err, "failed to put object to S3")
		}
	}
	return nil
}

type awsLogger struct {
	logger *slog.Logger
}

func newAWSLogger(logger *slog.Logger) *awsLogger {
	return &awsLogger{logger}
}
func (a *awsLogger) Logf(classification logging.Classification, format string, args ...any) {
	a.logger.Debug(fmt.Sprintf("[%v] %s", classification, fmt.Sprintf(format, args...)))
}
