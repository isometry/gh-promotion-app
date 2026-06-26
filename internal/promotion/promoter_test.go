// Package promoter_test provides a set of tests for the promotion package.
package promotion_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-github/v84/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	promotionPathProperty = "gitops-promotion-path"

	mainStage       = "main"
	stagingStage    = "staging"
	canaryStage     = "canary"
	productionStage = "production"

	mainRef       = "refs/heads/main"
	stagingRef    = "refs/heads/staging"
	canaryRef     = "refs/heads/canary"
	productionRef = "refs/heads/production"
)

func TestStageIndex(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected int
	}{
		{
			Name:     stagingStage,
			Input:    stagingRef,
			Expected: 1,
		},
		{
			Name:     canaryStage,
			Input:    canaryRef,
			Expected: 2,
		},
		{
			Name:     productionStage,
			Input:    productionRef,
			Expected: 3,
		},
		{
			Name:     "invalid_stage",
			Input:    "refs/heads/feature",
			Expected: -1,
		},
	}

	promoter := promotion.NewStagePromoter("test", []string{mainStage, stagingStage, canaryStage, productionStage})
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
			HeadRef:        mainRef,
			BaseRef:        stagingRef,
			ValidPromotion: true,
		},
		{
			Name:           "staging_to_canary",
			HeadRef:        stagingRef,
			BaseRef:        canaryRef,
			ValidPromotion: true,
		},
		{
			Name:           "canary_to_production",
			HeadRef:        canaryRef,
			BaseRef:        productionRef,
			ValidPromotion: true,
		},
		{
			Name:           "invalid_stage",
			HeadRef:        "refs/heads/feature",
			BaseRef:        productionRef,
			ValidPromotion: false,
		},
		{
			Name:           "invalid_order",
			HeadRef:        canaryRef,
			BaseRef:        mainRef,
			ValidPromotion: false,
		},
	}

	promoter := promotion.NewStagePromoter("test", []string{mainStage, stagingStage, canaryStage, productionStage})
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
			Ref:            mainRef,
			ExpectedStage:  stagingStage,
			ExpectedResult: true,
		},
		{
			Name:           "staging_to_canary",
			Ref:            stagingRef,
			ExpectedStage:  canaryStage,
			ExpectedResult: true,
		},
		{
			Name:           "canary_to_production",
			Ref:            canaryRef,
			ExpectedStage:  productionStage,
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
			Ref:            productionRef,
			ExpectedStage:  "",
			ExpectedResult: false,
		},
	}

	promoter := promotion.NewStagePromoter("test", []string{mainStage, stagingStage, canaryStage, productionStage})
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			stage, result := promoter.IsPromotableRef(tc.Ref)
			assert.Equal(t, tc.ExpectedStage, stage)
			assert.Equal(t, tc.ExpectedResult, result)
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
				promotionPathProperty: `main,staging,canary,production`,
			},
			PromoterKey:    promotionPathProperty,
			ExpectedStages: []string{mainStage, stagingStage, canaryStage, productionStage},
		},
		{
			Name: "valid_dynamic_promoter_2",
			Properties: map[string]string{
				promotionPathProperty: `develop,main,staging,canary,production`,
			},
			PromoterKey:    promotionPathProperty,
			ExpectedStages: []string{"develop", mainStage, stagingStage, canaryStage, productionStage},
		},
		{
			Name: "valid_dynamic_promoter_single_stage",
			Properties: map[string]string{
				promotionPathProperty: mainStage,
			},
			PromoterKey:    promotionPathProperty,
			ExpectedStages: []string{mainStage},
		},
		{
			Name: "invalid_dynamic_promoter",
			Properties: map[string]string{
				promotionPathProperty: `main,staging,canary,production`,
			},
		},
		{
			Name:        "missing_promoter_key",
			Properties:  map[string]string{},
			PromoterKey: promotionPathProperty,
		},
		{
			Name: "valid_trailing_comma",
			Properties: map[string]string{
				promotionPathProperty: `main,develop,`,
			},
			PromoterKey:    promotionPathProperty,
			ExpectedStages: []string{mainStage, "develop"},
		},
		{
			Name: "empty_path",
			Properties: map[string]string{
				promotionPathProperty: ``,
			},
			PromoterKey: promotionPathProperty,
		},
		{
			Name: "mismatched_promoter_key",
			Properties: map[string]string{
				promotionPathProperty: `main,staging,canary,production`,
			},
			PromoterKey: "gitops-promotion-path--invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			promoter := promotion.NewDynamicPromoter(helpers.NewNoopLogger(), testCustomProperties(t, tc.Properties), tc.PromoterKey, "test")
			if tc.ExpectedStages != nil {
				assert.Equal(t, tc.ExpectedStages, promoter.Stages)
			} else {
				assert.Equal(t, config.Promotion.DefaultStages, promoter.Stages)
			}
		})
	}
}

func testCustomProperties(t *testing.T, props map[string]string) models.CustomProperties {
	t.Helper()

	customProperties := make(models.CustomProperties, len(props))
	for key, value := range props {
		rawValue, err := json.Marshal(value)
		require.NoError(t, err)
		customProperties[key] = rawValue
	}
	return customProperties
}
