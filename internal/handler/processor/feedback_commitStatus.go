package processor //nolint:dupl // Processor implementations are similar

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type commitStatusFeedbackProcessor struct {
	logger           *slog.Logger
	githubController *github.Controller
}

// NewCommitStatusFeedbackProcessor creates a new processor for handling feedback from commit status checks.
func NewCommitStatusFeedbackProcessor(githubController *github.Controller, opts ...Option) Processor {
	_inst := &commitStatusFeedbackProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *commitStatusFeedbackProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("feedback-processor:commit-status")
}

func (p *commitStatusFeedbackProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing commit-status feedback...")
	bus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}

	if !config.Promotion.Feedback.CommitStatus.Enabled {
		p.logger.Debug("commit-status feedback is not enabled. skipping...")
		return bus, nil
	}

	if bus.EventStatus == promotion.Skipped {
		return bus, nil
	}

	// Automatically set the status to failure if an error occurred
	var status github.CommitStatus
	if bus.Error != nil {
		status = github.CommitStatusFailure
	}

	if status != github.CommitStatusFailure {
		switch bus.EventStatus { //nolint:exhaustive // Handled prior to switch to short-circuit
		case promotion.Success:
			status = github.CommitStatusSuccess
		case promotion.Failure:
			status = github.CommitStatusFailure
		case promotion.Error:
			status = github.CommitStatusError
		case promotion.Pending:
			status = github.CommitStatusPending
		}
	}

	if statusErr := p.githubController.SendPromotionFeedbackCommitStatus(bus, status); statusErr != nil {
		p.logger.Error("failed to send feedback commit-status", slog.Any("error", statusErr))
	}

	return
}
