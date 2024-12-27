package handler

// NoEventTypeError represents an error indicating that no event type was found.
type NoEventTypeError struct{}

func (m *NoEventTypeError) Error() string {
	return "no event type found"
}

// NoCredentialsError indicates that no credentials were provided or found when required for an operation.
type NoCredentialsError struct{}

func (m *NoCredentialsError) Error() string {
	return "no credentials found"
}

// NoInstallationIDError represents an error indicating that an installation ID is missing or not found.
type NoInstallationIDError struct{}

func (m *NoInstallationIDError) Error() string {
	return "no installation id found"
}
