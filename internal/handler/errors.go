package handler

type NoEventTypeError struct{}

func (m *NoEventTypeError) Error() string {
	return "no event type found"
}

type NoCredentialsError struct{}

func (m *NoCredentialsError) Error() string {
	return "no credentials found"
}

type NoInstallationIdError struct{}

func (m *NoInstallationIdError) Error() string {
	return "no installation id found"
}
