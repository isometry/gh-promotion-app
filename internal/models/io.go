package models

type Request struct {
	Body    string
	Headers map[string]string
}

type Response struct {
	Body       string
	Headers    map[string]string
	StatusCode int
}
