package helpers

//
//type Request struct {
//	Body    string
//	Headers map[string]string // lowercase keys to match AWS Lambda proxy request
//}
//
//type Response struct {
//	Body       string
//	StatusCode int
//}
//

import (
	"github.com/aws/aws-lambda-go/events"
)

type (
	Request  = events.APIGatewayV2HTTPRequest
	Response = events.APIGatewayV2HTTPResponse
)
