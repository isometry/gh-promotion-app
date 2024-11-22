package promotion_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/google/go-github/v66/github"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/stretchr/testify/assert"
)

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
			assert.Equal(t, tc.Expected, promotion.NewDefaultPromoter().StageIndex(tc.Input))
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
		{
			Name:           "invalid_order",
			HeadRef:        "refs/heads/canary",
			BaseRef:        "refs/heads/main",
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
			assert.Equal(t, tc.ValidPromotion, promotion.NewDefaultPromoter().IsPromotionRequest(pr))
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
		{
			Name:           "invalid_next_stage",
			Ref:            "refs/heads/production",
			ExpectedStage:  "",
			ExpectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			stage, result := promotion.NewDefaultPromoter().IsPromotableRef(tc.Ref)
			assert.Equal(t, tc.ExpectedStage, stage)
			assert.Equal(t, tc.ExpectedResult, result)
		})
	}
}

var discardLogger = slog.New(slog.NewJSONHandler(io.Discard, nil))

func TestNewDynamicPromoter(t *testing.T) {
	testCases := []struct {
		Name           string
		Properties     map[string]string
		PromoterKey    string
		ExpectedStages []string
	}{
		{
			Name: "valid_dynamic_promoter_1",
			Properties: map[string]string{
				"gitops-promotion-path": `main,staging,canary,production`,
			},
			PromoterKey:    "gitops-promotion-path",
			ExpectedStages: []string{"main", "staging", "canary", "production"},
		},
		{
			Name: "valid_dynamic_promoter_2",
			Properties: map[string]string{
				"gitops-promotion-path": `develop,main,staging,canary,production`,
			},
			PromoterKey:    "gitops-promotion-path",
			ExpectedStages: []string{"develop", "main", "staging", "canary", "production"},
		},
		{
			Name: "valid_dynamic_promoter_single_stage",
			Properties: map[string]string{
				"gitops-promotion-path": `main`,
			},
			PromoterKey:    "gitops-promotion-path",
			ExpectedStages: []string{"main"},
		},
		{
			Name: "invalid_dynamic_promoter",
			Properties: map[string]string{
				"gitops-promotion-path": `main,staging,canary,production`,
			},
		},
		{
			Name:        "missing_promoter_key",
			Properties:  map[string]string{},
			PromoterKey: "gitops-promotion-path",
		},
		{
			Name: "valid_trailing_comma",
			Properties: map[string]string{
				"gitops-promotion-path": `main,develop,`,
			},
			PromoterKey:    "gitops-promotion-path",
			ExpectedStages: []string{"main", "develop"},
		},
		{
			Name: "empty_path",
			Properties: map[string]string{
				"gitops-promotion-path": ``,
			},
			PromoterKey: "gitops-promotion-path",
		},
		{
			Name: "mismatched_promoter_key",
			Properties: map[string]string{
				"gitops-promotion-path": `main,staging,canary,production`,
			},
			PromoterKey: "gitops-promotion-path--invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			promoter := promotion.NewDynamicPromoter(discardLogger, tc.Properties, tc.PromoterKey)
			if tc.ExpectedStages != nil {
				assert.Equal(t, tc.ExpectedStages, promoter.Stages)
			} else {
				assert.Equal(t, promotion.DefaultStages, promoter.Stages)
			}
		})
	}
}
