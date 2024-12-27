package promotion

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/google/go-github/v67/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
)

var (
	// DefaultStages holds the default promotion stages
	DefaultStages = []string{
		"main",
		"staging",
		"canary",
		"production",
	}
)

// Promoter is a struct that holds the promotion stages
type Promoter struct {
	Stages []string
}

// _defaultPromoter is NewDefaultPromoter instance cached at runtime
var _defaultPromoter = NewDefaultPromoter()

// NewDefaultPromoter creates a new default promoter instance
func NewDefaultPromoter() *Promoter {
	return NewStagePromoter(DefaultStages)
}

// NewStagePromoter creates a new promoter instance with the given stages
func NewStagePromoter(stages []string) *Promoter {
	return &Promoter{stages}
}

// NewDynamicPromoter creates a new promoter instance with the given stages
func NewDynamicPromoter(logger *slog.Logger, props map[string]string, promoterKey string) *Promoter {
	stagesBlob, found := props[promoterKey]
	stagesBlob = strings.TrimSpace(stagesBlob)
	if !found {
		logger.Warn("promoter key not found in properties. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}
	if stagesBlob == "" {
		logger.Warn("promoter key found but empty. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}

	if strings.HasSuffix(stagesBlob, ",") {
		logger.Warn("promoter key found but trailing comma found. Removing...", slog.Any("key", promoterKey))
		stagesBlob = strings.TrimSuffix(stagesBlob, ",")
	}

	stages := strings.Split(stagesBlob, ",")
	if len(stages) == 0 {
		logger.Warn("promoter key found but no stages were defined. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}
	for i, stage := range stages {
		stages[i] = strings.TrimSpace(stage)
	}
	logger.Debug("dynamic promoter stages loaded...", slog.Any("stages", stages))
	return NewStagePromoter(stages)
}

// StageIndex returns the index of the given ref in the promotion Stages
func (sp *Promoter) StageIndex(ref string) int {
	// find the index of the head ref in the promotion Stages
	// -1 indicates that the head ref is not a promotion stage
	return slices.Index(sp.Stages, helpers.NormaliseRef(ref))
}

// IsPromotionRequest checks if the given pull request is a promotion request
func (sp *Promoter) IsPromotionRequest(pr *github.PullRequest) bool {
	// ensure p.HeadRef and baseRef are contiguous promotion Stages, and that the head ref is not the last stage
	if headIndex := sp.StageIndex(*pr.Head.Ref); headIndex != -1 && headIndex < len(sp.Stages)-1 {
		return sp.Stages[headIndex+1] == helpers.NormaliseRef(*pr.Base.Ref)
	}

	return false
}

// IsPromotableRef checks if the given ref is promotable
func (sp *Promoter) IsPromotableRef(ref string) (string, bool) {
	if headIndex := sp.StageIndex(ref); headIndex != -1 && headIndex < len(sp.Stages)-1 {
		return sp.Stages[headIndex+1], true
	}

	return "", false
}
