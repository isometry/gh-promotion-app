package promotion

type NoPromotionRequestError struct{}

func (m *NoPromotionRequestError) Error() string {
	return "no pull request found"
}
