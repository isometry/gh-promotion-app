package controllers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/pkg/errors"
	"log/slog"
	"os"
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

func WithConfig(cfg *aws.Config) Option {
	return func(c *AWS) {
		c.config = cfg
	}
}

func WithContext(ctx context.Context) Option {
	return func(a *AWS) {
		a.ctx = ctx
	}
}

func NewAWSController(opts ...Option) (*AWS, error) {
	_inst := &AWS{}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.config == nil {
		cfg, err := config.LoadDefaultConfig(_inst.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load AWS configuration")
		}
		_inst.config = &cfg
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})).With("controller", "aws")
	}

	_inst.s3Client = s3.NewFromConfig(*_inst.config)
	_inst.ssmClient = ssm.NewFromConfig(*_inst.config)
	return _inst, nil
}

func (a *AWS) RetrieveCredentials(ctx context.Context, key string, encrypted bool) (*string, error) {
	ssmResponse, err := a.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(os.Getenv(key)),
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
