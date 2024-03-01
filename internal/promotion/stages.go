package promotion

import (
	"slices"
	"strings"

	"github.com/google/go-github/v59/github"
)

var (
	Stages = []string{
		"main",
		"staging",
		"canary",
		"production",
	}
)

func StageRef(stage string) string {
	return "refs/heads/" + stage
}

func StageName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func StageIndex(ref string) int {
	// find the index of the head ref in the promotion stages
	// -1 indicates that the head ref is not a promotion stage
	return slices.Index(Stages, StageName(ref))
}

func IsPromotionRequest(pr *github.PullRequest) bool {
	// ensure p.HeadRef and baseRef are contiguous promotion stages, and that the head ref is not the last stage
	if headIndex := StageIndex(*pr.Head.Ref); headIndex != -1 && headIndex < len(Stages)-1 {
		return Stages[headIndex+1] == StageName(*pr.Base.Ref)
	}

	return false
}

func IsPromoteableRef(ref string) bool {
	if headIndex := StageIndex(ref); headIndex != -1 && headIndex < len(Stages)-1 {
		return true
	}

	return false
}
