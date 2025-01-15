package helpers

import (
	"encoding/json"
	"net/http"

	"github.com/isometry/gh-promotion-app/internal/models"
)

type httpResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// RespondHTTP writes the response to the http.ResponseWriter.
func RespondHTTP(rw http.ResponseWriter, response models.Response, err error) {
	hR := httpResponse{
		Message: response.Body,
	}
	if err != nil {
		hR.Error = err.Error()
	}

	respBody, _ := json.Marshal(hR) //nolint:errchkjson // Errors can be safely ignored in this context
	statusCode := response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	rw.WriteHeader(statusCode)
	for k, v := range response.Headers {
		rw.Header().Set(k, v)
	}
	_, _ = rw.Write(respBody)
}
