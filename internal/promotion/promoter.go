package promotion

import (
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"log/slog"
	"slices"
	"strings"
)

var (
	DefaultStages = []string{
		"main",
		"staging",
		"canary",
		"production",
	}
)

type Promoter struct {
	Stages []string
}

type dynamicPromoterProperties struct {
	Initiator string `json:"initiator"`
}

// _defaultPromoter is NewDefaultPromoter instance cached at runtime
var _defaultPromoter = NewDefaultPromoter()

func NewDefaultPromoter() *Promoter {
	return NewStagePromoter(DefaultStages)
}
func NewStagePromoter(stages []string) *Promoter {
	return &Promoter{stages}
}

func NewDynamicPromoter(logger *slog.Logger, props map[string]string, promoterKey string) *Promoter {
	stagesBlob, found := props[promoterKey]
	if !found {
		logger.Warn("Promoter key not found in properties. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}
	if stagesBlob == "" {
		logger.Warn("Promoter key found but empty. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}

	if strings.HasSuffix(stagesBlob, ",") {
		logger.Warn("Promoter key found but trailing comma found. Removing...", slog.Any("key", promoterKey))
		stagesBlob = strings.TrimSuffix(stagesBlob, ",")
	}

	stages := strings.Split(stagesBlob, ",")
	if len(stages) == 0 {
		logger.Warn("Promoter key found but no stages were defined. Defaulting to standard promoter...", slog.Any("key", promoterKey))
		return _defaultPromoter
	}
	logger.Debug("Dynamic promoter stages loaded...", slog.Any("stages", stages))
	return NewStagePromoter(stages)
}

func (sp *Promoter) StageIndex(ref string) int {
	// find the index of the head ref in the promotion Stages
	// -1 indicates that the head ref is not a promotion stage
	return slices.Index(sp.Stages, helpers.NormaliseRef(ref))
}

func (sp *Promoter) IsPromotionRequest(pr *github.PullRequest) bool {
	// ensure p.HeadRef and baseRef are contiguous promotion Stages, and that the head ref is not the last stage
	if headIndex := sp.StageIndex(*pr.Head.Ref); headIndex != -1 && headIndex < len(sp.Stages)-1 {
		return sp.Stages[headIndex+1] == helpers.NormaliseRef(*pr.Base.Ref)
	}

	return false
}

func (sp *Promoter) IsPromotableRef(ref string) (string, bool) {
	if headIndex := sp.StageIndex(ref); headIndex != -1 && headIndex < len(sp.Stages)-1 {
		return sp.Stages[headIndex+1], true
	}

	return "", false
}
