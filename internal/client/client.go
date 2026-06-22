package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Tokener provides a valid Bearer token on demand.
type Tokener interface {
	Token(ctx context.Context) (string, error)
}

// Client wraps the Webhookr SVC REST API (v1).
type Client struct {
	baseURL    string
	auth       Tokener
	httpClient *http.Client
}

// New creates a Client for the given SVC base URL.
func New(baseURL string, auth Tokener) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		auth:       auth,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Do executes an authenticated request against the SVC.
//
//   - body is JSON-serialised when non-nil.
//   - out is JSON-decoded from the response when non-nil and the status is < 400.
//
// Returns the HTTP status code so callers can handle 404 (resource deleted outside
// Terraform) without treating it as a fatal error.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) (int, error) {
	token, err := c.auth.Token(ctx)
	if err != nil {
		return 0, fmt.Errorf("obtaining auth token: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader) //nolint:gosec // G107: baseURL is administrator-supplied provider config
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decoding response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
