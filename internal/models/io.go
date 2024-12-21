// Package models provides the core data structures for handling webhook requests and responses.
package models

// Request represents an incoming client request containing a body and associated headers.
type Request struct {
	Body    string
	Headers map[string]string
}

// Response defines the structure for an HTTP response containing a body, headers, and a status code.
type Response struct {
	Body       string
	Headers    map[string]string
	StatusCode int
}
