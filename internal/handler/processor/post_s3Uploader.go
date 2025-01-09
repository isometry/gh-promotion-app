package processor

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/controllers/aws"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type s3UploaderPostProcessor struct {
	logger        *slog.Logger
	awsController *aws.Controller
}

// NewS3UploaderPostProcessor constructs a Processor instance for handling S3 upload post-processing with optional configurations.
func NewS3UploaderPostProcessor(awsController *aws.Controller, opts ...Option) Processor {
	_inst := &s3UploaderPostProcessor{awsController: awsController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *s3UploaderPostProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("post-processor:s3-uploader")
}

func (p *s3UploaderPostProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected S3UploadRequest got %T", req)
	}
	bus = parsedBus
	body := parsedBus.Body

	s3cap := config.Global.S3.Upload
	if !s3cap.Enabled {
		p.logger.Debug("s3 upload is disabled")
		return bus, nil
	}

	p.logger.Debug("processing s3 upload...")

	if bus.Context.Owner == nil || bus.Context.Repository == nil {
		msg := "missing owner or repository context"
		p.logger.Error(msg)
		return bus, promotion.NewInternalError(msg)
	}

	s3cap.BucketName = strings.TrimSpace(s3cap.BucketName)
	if s3cap.BucketName == "" {
		p.logger.Error("s3 bucket name was left empty but upload is enabled")
		return bus, nil
	}

	entryID := fmt.Sprintf("%s/%s/%s", *bus.Context.Owner, *bus.Context.Repository, bus.EventType)
	if err = p.awsController.PutS3Object(entryID, s3cap.BucketName, body); err != nil {
		p.logger.Warn("failed to store event in S3", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	return bus, nil
}
