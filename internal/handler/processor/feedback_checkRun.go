package processor //nolint:dupl // Processor implementations are similar

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type checkRunFeedbackProcessor struct {
	logger           *slog.Logger
	githubController *github.Controller
}

// NewCheckRunFeedbackProcessor creates a new processor for handling feedback from check run events.
func NewCheckRunFeedbackProcessor(githubController *github.Controller, opts ...Option) Processor {
	_inst := &checkRunFeedbackProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (c *checkRunFeedbackProcessor) SetLogger(logger *slog.Logger) {
	c.logger = logger.WithGroup("feedback-processor:check-run")
}

func (c *checkRunFeedbackProcessor) Process(req any) (bus *promotion.Bus, err error) {
	c.logger.Debug("processing check-run feedback...")
	bus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}

	if !config.Promotion.Feedback.CheckRun.Enabled {
		c.logger.Debug("check-run feedback is not enabled. skipping...")
		return bus, nil
	}

	if bus.EventStatus == promotion.Skipped {
		return bus, nil
	}

	// Automatically set the conclusion to failure if an error occurred
	conclusion := github.CheckRunConclusionNeutral
	if bus.EventStatus == promotion.Success {
		conclusion = github.CheckRunConclusionSuccess
	}

	if statusErr := c.githubController.SendPromotionFeedbackCheckRun(bus, conclusion); statusErr != nil {
		c.logger.Error("failed to send feedback check-run", slog.Any("error", statusErr))
	}
	return
}
