package processor

import (
	"log/slog"
	"net/http"

	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

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

	targetStages, isRollback := bus.Context.Promoter.IsRollbackRef(*e.Ref, config.Promotion.Rollback.Prefix)
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
	case "behind", "diverged":
		msg := "rollback rejected: rollback branch is not behind the target stage"
		p.logger.Error(msg, slog.String("status", comparison.GetStatus()))
		bus.Response = models.Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}
		return bus, promotion.NewInternalError(msg)
	case "identical":
		p.logger.Info("rollback branch is identical to primary stage, nothing to do")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	// Roll back all target stages to the rollback commit
	for _, stage := range targetStages {
		stageLogger := p.logger.With(slog.String("stage", stage))
		bus.Context.BaseRef = helpers.NormaliseFullRefPtr(stage)

		if err = p.githubController.ForceUpdateRefToSha(bus.Context); err != nil {
			stageLogger.Error("failed to force update ref", slog.Any("error", err))
			bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
			return bus, err
		}
		stageLogger.Info("rollback complete", slog.String("headSHA", *e.After))
	}

	bus.EventStatus = promotion.Skipped
	return bus, nil
}
