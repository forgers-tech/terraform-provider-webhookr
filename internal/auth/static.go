package auth

import "context"

// StaticTokener implements client.Tokener with a fixed Bearer token.
// Used when the provider is configured with an api_token instead of Firebase credentials.
type StaticTokener struct {
	token string
}

// NewStaticTokener creates a StaticTokener for the given token.
func NewStaticTokener(token string) *StaticTokener {
	return &StaticTokener{token: token}
}

// Token returns the static token, satisfying the Tokener interface.
func (s *StaticTokener) Token(_ context.Context) (string, error) {
	return s.token, nil
}
