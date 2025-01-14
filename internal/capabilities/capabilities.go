// Package capabilities provides the global capabilities and promotion features available to the application.
package capabilities

// Global is a struct that contains the global capabilities.
var Global = struct {
	// S3 is a struct that contains the capabilities available when interacting with S3.
	S3 struct {
		Upload struct {
			BucketName string
			Enabled    bool
		}
	}
}{}

// Promotion is a struct that contains the capabilities of the promotion features.
var Promotion = struct {
	// DynamicPromotion is a struct that contains the capabilities available when processing dynamic promotions.
	DynamicPromotion struct {
		Enabled bool
		Key     string
	}
	// Push is a struct that contains the capabilities available when processing push events.
	Push struct {
		CreateTargetRef bool
	}
	// Feedback is a struct that contains the feedback capabilities.
	Feedback struct {
		Enabled bool
		Context string
	}
}{}
