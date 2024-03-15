package promotion

import (
	"slices"
	"strings"

	"github.com/google/go-github/v59/github"
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

func StageRef(stage string) *string {
	ref := "refs/heads/" + strings.TrimPrefix(stage, "refs/heads/")
	return &ref
}

func StageName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func (sp *Promoter) StageIndex(ref string) int {
	// find the index of the head ref in the promotion stages
	// -1 indicates that the head ref is not a promotion stage
	return slices.Index(sp.stages, StageName(ref))
}

func (sp *Promoter) IsPromotionRequest(pr *github.PullRequest) bool {
	// ensure p.HeadRef and baseRef are contiguous promotion stages, and that the head ref is not the last stage
	if headIndex := sp.StageIndex(*pr.Head.Ref); headIndex != -1 && headIndex < len(sp.stages)-1 {
		return sp.stages[headIndex+1] == StageName(*pr.Base.Ref)
	}

	return false
}

func (sp *Promoter) IsPromotableRef(ref string) (string, bool) {
	if headIndex := sp.StageIndex(ref); headIndex != -1 && headIndex < len(sp.stages)-1 {
		return sp.stages[headIndex+1], true
	}

	return "", false
}
