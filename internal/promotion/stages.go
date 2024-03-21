package promotion

import (
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"slices"
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
	stages []string
}

func NewStagePromoter(stages []string) *Promoter {
	return &Promoter{stages}
}

func (sp *Promoter) StageIndex(ref string) int {
	// find the index of the head ref in the promotion stages
	// -1 indicates that the head ref is not a promotion stage
	return slices.Index(sp.stages, helpers.NormaliseRef(ref))
}

func (sp *Promoter) IsPromotionRequest(pr *github.PullRequest) bool {
	// ensure p.HeadRef and baseRef are contiguous promotion stages, and that the head ref is not the last stage
	if headIndex := sp.StageIndex(*pr.Head.Ref); headIndex != -1 && headIndex < len(sp.stages)-1 {
		return sp.stages[headIndex+1] == helpers.NormaliseRef(*pr.Base.Ref)
	}

	return false
}

func (sp *Promoter) IsPromotableRef(ref string) (string, bool) {
	if headIndex := sp.StageIndex(ref); headIndex != -1 && headIndex < len(sp.stages)-1 {
		return sp.stages[headIndex+1], true
	}

	return "", false
}
