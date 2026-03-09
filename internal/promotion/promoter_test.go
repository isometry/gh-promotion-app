// Package promoter_test provides a set of tests for the promotion package.
package promotion_test

import (
	"testing"

	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/helpers"
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

	promoter := promotion.NewStagePromoter("test", []string{"main", "staging", "canary", "production"})
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, promoter.StageIndex(tc.Input))
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

	promoter := promotion.NewStagePromoter("test", []string{"main", "staging", "canary", "production"})
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
			assert.Equal(t, tc.ValidPromotion, promoter.IsPromotionRequest(pr))
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

	promoter := promotion.NewStagePromoter("test", []string{"main", "staging", "canary", "production"})
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			stage, result := promoter.IsPromotableRef(tc.Ref)
			assert.Equal(t, tc.ExpectedStage, stage)
			assert.Equal(t, tc.ExpectedResult, result)
		})
	}
}

func TestIsRollbackRef(t *testing.T) {
	testCases := []struct {
		Name           string
		Stages         []string
		Ref            string
		Prefix         string
		CascadeStages  []string
		ExpectedStages []string
		ExpectedOk     bool
	}{
		{
			Name:           "rollback_last_stage_cascades_canary",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: []string{"production", "canary"},
			ExpectedOk:     true,
		},
		{
			Name:           "rollback_last_stage_short_ref_cascades_canary",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: []string{"production", "canary"},
			ExpectedOk:     true,
		},
		{
			Name:           "rollback_non_last_stage",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/rollback-canary",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: nil,
			ExpectedOk:     false,
		},
		{
			Name:          "rollback_first_stage",
			Stages:        []string{"main", "staging", "canary", "production"},
			Ref:           "refs/heads/rollback-main",
			Prefix:        "rollback-",
			CascadeStages: []string{"canary"},
			ExpectedStages: nil,
			ExpectedOk:     false,
		},
		{
			Name:           "non_rollback_branch",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/feature",
			Prefix:         "rollback-",
			ExpectedStages: nil,
			ExpectedOk:     false,
		},
		{
			Name:           "promotable_ref",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/main",
			Prefix:         "rollback-",
			ExpectedStages: nil,
			ExpectedOk:     false,
		},
		{
			Name:           "custom_prefix_cascades_canary",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/revert-production",
			Prefix:         "revert-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: []string{"production", "canary"},
			ExpectedOk:     true,
		},
		{
			Name:           "single_stage_promoter",
			Stages:         []string{"main"},
			Ref:            "rollback-main",
			Prefix:         "rollback-",
			ExpectedStages: []string{"main"},
			ExpectedOk:     true,
		},
		{
			Name:           "three_stages_with_canary_cascades",
			Stages:         []string{"main", "canary", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: []string{"production", "canary"},
			ExpectedOk:     true,
		},
		{
			Name:           "three_stages_without_canary_no_cascade",
			Stages:         []string{"main", "staging", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary"},
			ExpectedStages: []string{"production"},
			ExpectedOk:     true,
		},
		{
			Name:           "two_stages_no_cascade",
			Stages:         []string{"main", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			ExpectedStages: []string{"production"},
			ExpectedOk:     true,
		},
		{
			Name:           "cascade_ignores_last_stage",
			Stages:         []string{"main", "staging", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"production"},
			ExpectedStages: []string{"production"},
			ExpectedOk:     true,
		},
		{
			Name:           "cascade_multiple_stages",
			Stages:         []string{"main", "staging", "canary", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"canary", "staging"},
			ExpectedStages: []string{"production", "canary", "staging"},
			ExpectedOk:     true,
		},
		{
			Name:           "cascade_unknown_stage_ignored",
			Stages:         []string{"main", "staging", "production"},
			Ref:            "refs/heads/rollback-production",
			Prefix:         "rollback-",
			CascadeStages:  []string{"nonexistent"},
			ExpectedStages: []string{"production"},
			ExpectedOk:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			promoter := promotion.NewStagePromoter("test", tc.Stages)
			stages, ok := promoter.IsRollbackRef(tc.Ref, tc.Prefix, tc.CascadeStages)
			assert.Equal(t, tc.ExpectedStages, stages)
			assert.Equal(t, tc.ExpectedOk, ok)
		})
	}
}

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
			promoter := promotion.NewDynamicPromoter(helpers.NewNoopLogger(), tc.Properties, tc.PromoterKey, "test")
			if tc.ExpectedStages != nil {
				assert.Equal(t, tc.ExpectedStages, promoter.Stages)
			} else {
				assert.Equal(t, config.Promotion.DefaultStages, promoter.Stages)
			}
		})
	}
}
