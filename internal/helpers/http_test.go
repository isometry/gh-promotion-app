package helpers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/stretchr/testify/assert"
)

type testCase struct {
	Name     string
	Response models.Response
	Error    error
	Expected expectedResponse
}

type expectedResponse struct {
	StatusCode int
	Body       string
	Header     string
}

func TestNewHttpResponse(t *testing.T) {
	testCases := []testCase{
		{
			Name: "with_valid_response_and_no_error",
			Response: models.Response{
				StatusCode: http.StatusOK,
				Body:       "Success",
				Headers:    map[string]string{"Content-Type": "application/json"},
			},
			Expected: expectedResponse{
				StatusCode: http.StatusOK,
				Body:       "Success",
				Header:     "application/json",
			},
		},
		{
			Name: "with_valid_response_and_error",
			Response: models.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       "Failure",
				Headers:    map[string]string{"Content-Type": "application/json"},
			},
			Error: errors.New("internal Server Error"),
			Expected: expectedResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       "Failure",
				Header:     "application/json",
			},
		},
		{
			Name:     "with_empty_response_and_no_error",
			Response: models.Response{},
			Expected: expectedResponse{
				StatusCode: http.StatusOK,
				Body:       "",
				Header:     "",
			},
		},
		{
			Name:     "with_empty_response_and_error",
			Response: models.Response{},
			Error:    errors.New("internal Server Error"),
			Expected: expectedResponse{
				StatusCode: http.StatusOK,
				Body:       "",
				Header:     "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rw := httptest.NewRecorder()

			helpers.RespondHTTP(tc.Response, tc.Error, rw)

			assert.Equal(t, tc.Expected.StatusCode, rw.Code)
			assert.Equal(t, tc.Expected.Header, rw.Header().Get("Content-Type"))
			assert.Contains(t, rw.Body.String(), tc.Expected.Body)
			if tc.Error != nil {
				assert.Contains(t, rw.Body.String(), tc.Error.Error())
			} else {
				assert.NotContains(t, rw.Body.String(), "error")
			}
		})
	}
}
