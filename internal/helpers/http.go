package helpers

import (
	"encoding/json"
	"net/http"
)

type httpResponse struct {
	Message string `json:"message"`
	Error   any    `json:"error"`
}

func NewHttpResponse(response Response, err error, rw http.ResponseWriter) {
	hR := httpResponse{
		Message: response.Body,
		Error:   err,
	}

	respBody, _ := json.Marshal(hR)
	rw.WriteHeader(response.StatusCode)
	for k, v := range response.Headers {
		rw.Header().Set(k, v)
	}
	_, _ = rw.Write(respBody)
}
