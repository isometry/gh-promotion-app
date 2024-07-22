package processor

import (
	"log/slog"
	"net/http"

	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type s3UploaderPostProcessor struct {
	logger        *slog.Logger
	awsController *controllers.AWS
}

func NewS3UploaderPostProcessor(awsController *controllers.AWS, opts ...Option) Processor {
	_inst := &s3UploaderPostProcessor{awsController: awsController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *s3UploaderPostProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:validator")
}

func (p *s3UploaderPostProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected S3UploadRequest got %T", req)
	}
	bus = parsedBus
	body := parsedBus.Body

	s3cap := capabilities.Global.S3.Upload
	if !s3cap.Enabled {
		p.logger.Debug("s3 upload is disabled")
		return bus, nil
	}

	if err = p.awsController.PutS3Object(bus.EventType, s3cap.BucketName, body); err != nil {
		p.logger.Warn("failed to store event in S3", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	return bus, nil
}
