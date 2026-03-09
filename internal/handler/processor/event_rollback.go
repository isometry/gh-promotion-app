package processor

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

const rollbackMaxRetries = 3

type rollbackEventProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewRollbackEventProcessor creates a rollback event processor with an optional configuration and attaches a Controller controller to it.
func NewRollbackEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &rollbackEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *rollbackEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:rollback")
}

func (p *rollbackEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing rollback event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus

	if !config.Promotion.Rollback.Enabled {
		p.logger.Debug("rollback is not enabled. skipping...")
		return bus, nil
	}

	e, ok := bus.Event.(*github.PushEvent)
	if !ok {
		return bus, nil
	}

	targetStages, isRollback := bus.Context.Promoter.IsRollbackRef(*e.Ref, config.Promotion.Rollback.Prefix, config.Promotion.Rollback.CascadeStages)
	if !isRollback {
		p.logger.Debug("ignoring push event on non-rollback branch", slog.String("ref", *e.Ref))
		return bus, nil
	}

	p.logger.Info("rollback detected", slog.String("ref", *e.Ref), slog.Any("targetStages", targetStages))

	rollbackRef := helpers.NormaliseRef(*e.Ref)
	bus.Context.HeadRef = e.Ref
	bus.Context.HeadSHA = e.After

	// Validate the rollback branch against the primary target before modifying any branch
	primaryStage := targetStages[0]
	comparison, err := p.githubController.CompareCommits(bus.Context, rollbackRef, primaryStage)
	if err != nil {
		p.logger.Error("failed to compare commits", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		return bus, err
	}

	switch comparison.GetStatus() {
	case "behind":
		msg := "rollback rejected: rollback branch is behind the target stage"
		p.logger.Error(msg, slog.String("status", comparison.GetStatus()))
		bus.Response = models.Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}
		return bus, promotion.NewInternalError(msg)
	case "diverged":
		msg := "rollback rejected: rollback branch has diverged from the target stage"
		p.logger.Error(msg, slog.String("status", comparison.GetStatus()))
		bus.Response = models.Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}
		return bus, promotion.NewInternalError(msg)
	case "identical":
		p.logger.Info("rollback branch is identical to primary stage, nothing to do")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	// Roll back all target stages to the rollback commit
	var succeeded []string
	for _, stage := range targetStages {
		stageLogger := p.logger.With(slog.String("stage", stage))
		bus.Context.BaseRef = helpers.NormaliseFullRefPtr(stage)

		_, err = backoff.Retry(context.Background(), func() (struct{}, error) {
			return struct{}{}, p.githubController.ForceUpdateRefToSha(bus.Context)
		},
			backoff.WithMaxTries(rollbackMaxRetries),
			backoff.WithNotify(func(err error, d time.Duration) {
				stageLogger.Warn("retrying force update ref", slog.Any("error", err), slog.Duration("backoff", d))
			}),
		)
		if err != nil {
			msg := fmt.Sprintf("partial rollback: succeeded=%v, failed=%s: %v", succeeded, stage, err)
			stageLogger.Error(msg)
			bus.Response = models.Response{Body: msg, StatusCode: http.StatusInternalServerError}
			return bus, promotion.NewInternalError(msg)
		}
		succeeded = append(succeeded, stage)
		stageLogger.Info("rollback complete", slog.String("headSHA", *e.After))
	}

	bus.EventStatus = promotion.Rollback
	return bus, nil
}
