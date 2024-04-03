package controllers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/logging"
	"github.com/pkg/errors"
	"io"
	"log/slog"
	"time"
)

type AWS struct {
	ctx    context.Context
	logger *slog.Logger

	config    *aws.Config
	s3Client  *s3.Client
	ssmClient *ssm.Client
}

type Option func(*AWS)

func NewAWSController(opts ...Option) (*AWS, error) {
	_inst := &AWS{}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	_inst.logger = _inst.logger.With("controller", "AWS")
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.config == nil {
		_inst.logger.Debug("loading default AWS configuration...")
		cfg, err := config.LoadDefaultConfig(_inst.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load AWS configuration")
		}
		cfg.Logger = newAWSLogger(_inst.logger)
		_inst.config = &cfg
	}

	_inst.s3Client = s3.NewFromConfig(*_inst.config)
	_inst.ssmClient = ssm.NewFromConfig(*_inst.config)
	return _inst, nil
}

func (a *AWS) GetSecret(key string, encrypted bool) (*string, error) {
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

func (a *AWS) PutS3Object(eventType, bucket string, body []byte) error {
	if bucket != "" {
		key := fmt.Sprintf("%s.%s", time.Now().UTC().Format(time.RFC3339Nano), eventType)
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
