package promotion_test

import (
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStageRef(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "full_ref_format",
			Input:    "refs/heads/main",
			Expected: "refs/heads/main",
		},
		{
			Name:     "short_ref_format",
			Input:    "main",
			Expected: "refs/heads/main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, *promotion.StageRef(tc.Input))
		})
	}
}

func TestStageName(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "full_ref_format",
			Input:    "refs/heads/main",
			Expected: "main",
		},
		{
			Name:     "short_ref_format",
			Input:    "main",
			Expected: "main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, promotion.StageName(tc.Input))
		})
	}
}

func TestStageIndex(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected int
	}{
		{
			Name:     "staging",
			Input:    "refs/heads/staging",
			Expected: 1,
		},
		{
			Name:     "canary",
			Input:    "refs/heads/canary",
			Expected: 2,
		},
		{
			Name:     "production",
			Input:    "refs/heads/production",
			Expected: 3,
		},
		{
			Name:     "invalid_stage",
			Input:    "refs/heads/feature",
			Expected: -1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, promotion.NewStagePromoter(promotion.DefaultStages).StageIndex(tc.Input))
		})
	}
}

func TestIsPromotionRequest(t *testing.T) {
	testCases := []struct {
		Name           string
		HeadRef        string
		BaseRef        string
		ValidPromotion bool
	}{
		{
			Name:           "main_to_staging",
			HeadRef:        "refs/heads/main",
			BaseRef:        "refs/heads/staging",
			ValidPromotion: true,
		},
		{
			Name:           "staging_to_canary",
			HeadRef:        "refs/heads/staging",
			BaseRef:        "refs/heads/canary",
			ValidPromotion: true,
		},
		{
			Name:           "canary_to_production",
			HeadRef:        "refs/heads/canary",
			BaseRef:        "refs/heads/production",
			ValidPromotion: true,
		},
		{
			Name:           "invalid_stage",
			HeadRef:        "refs/heads/feature",
			BaseRef:        "refs/heads/production",
			ValidPromotion: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			pr := &github.PullRequest{
				Head: &github.PullRequestBranch{
					Ref: &tc.HeadRef,
				},
				Base: &github.PullRequestBranch{
					Ref: &tc.BaseRef,
				},
			}
			assert.Equal(t, tc.ValidPromotion, promotion.NewStagePromoter(promotion.DefaultStages).IsPromotionRequest(pr))
		})
	}
}

func TestIsPromotableRef(t *testing.T) {
	testCases := []struct {
		Name           string
		Ref            string
		ExpectedStage  string
		ExpectedResult bool
	}{
		{
			Name:           "main_to_staging",
			Ref:            "refs/heads/main",
			ExpectedStage:  "staging",
			ExpectedResult: true,
		},
		{
			Name:           "staging_to_canary",
			Ref:            "refs/heads/staging",
			ExpectedStage:  "canary",
			ExpectedResult: true,
		},
		{
			Name:           "canary_to_production",
			Ref:            "refs/heads/canary",
			ExpectedStage:  "production",
			ExpectedResult: true,
		},
		{
			Name:           "invalid_stage",
			Ref:            "refs/heads/feature",
			ExpectedStage:  "",
			ExpectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			stage, result := promotion.NewStagePromoter(promotion.DefaultStages).IsPromotableRef(tc.Ref)
			assert.Equal(t, tc.ExpectedStage, stage)
			assert.Equal(t, tc.ExpectedResult, result)
		})
	}
}
