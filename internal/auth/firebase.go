package auth

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	customTokenAudience = "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit" // #nosec G101 -- Google API audience URL, not a credential
	signInURL           = "https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken"
	refreshURL          = "https://securetoken.googleapis.com/v1/token"

	// providerUID is the Firebase Auth UID that represents the Terraform provider.
	// It is created automatically on first sign-in with a custom token.
	providerUID = "terraform-provider-webhookr"

	tokenLifetime      = time.Hour
	tokenRefreshBefore = 5 * time.Minute
)

// FirebaseClient authenticates against the Webhookr SVC using a Firebase service
// account. It mints a Custom Token (RS256 JWT signed with the service account
// private key), exchanges it for a short-lived Firebase ID token, and refreshes
// transparently before expiry.
type FirebaseClient struct {
	apiKey              string
	serviceAccountEmail string
	privateKey          *rsa.PrivateKey
	httpClient          *http.Client

	mu           sync.Mutex
	idToken      string
	refreshToken string
	expiresAt    time.Time
}

// New creates a FirebaseClient. privateKeyPEM may use literal \n escapes (as
// produced when a PEM key is stored in a JSON-encoded Terraform variable) —
// they are normalised automatically.
func New(apiKey, serviceAccountEmail, privateKeyPEM string) (*FirebaseClient, error) {
	key, err := parseRSAKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing service_account_key: %w", err)
	}
	return &FirebaseClient{
		apiKey:              apiKey,
		serviceAccountEmail: serviceAccountEmail,
		privateKey:          key,
		httpClient:          &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Token returns a valid Firebase ID token, transparently refreshing when needed.
// Safe for concurrent use.
func (c *FirebaseClient) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.idToken != "" && time.Until(c.expiresAt) > tokenRefreshBefore {
		return c.idToken, nil
	}
	if c.refreshToken != "" {
		if err := c.refresh(ctx); err == nil {
			return c.idToken, nil
		}
		// Fall through to a full sign-in if the refresh token has also expired.
	}
	if err := c.signIn(ctx); err != nil {
		return "", err
	}
	return c.idToken, nil
}

func (c *FirebaseClient) signIn(ctx context.Context) error {
	customToken, err := c.mintCustomToken()
	if err != nil {
		return fmt.Errorf("minting custom token: %w", err)
	}

	payload, err := json.Marshal(map[string]any{"token": customToken, "returnSecureToken": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		signInURL+"?key="+url.QueryEscape(c.apiKey), //nolint:gosec // G107: URL is a fixed Google endpoint with a query-escaped key
		bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sign-in request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result struct {
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		Error        *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding sign-in response: %w", err)
	}
	if result.Error != nil {
		return fmt.Errorf("firebase sign-in error: %s", result.Error.Message)
	}
	c.idToken = result.IDToken
	c.refreshToken = result.RefreshToken
	c.expiresAt = time.Now().Add(tokenLifetime)
	return nil
}

func (c *FirebaseClient) refresh(ctx context.Context) error {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		refreshURL+"?key="+url.QueryEscape(c.apiKey), //nolint:gosec // G107: URL is a fixed Google endpoint with a query-escaped key
		strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh returned status %d", resp.StatusCode)
	}

	var result struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	c.idToken = result.IDToken
	c.refreshToken = result.RefreshToken
	c.expiresAt = time.Now().Add(tokenLifetime)
	return nil
}

// mintCustomToken creates a Firebase Custom Token signed with the service account
// private key using RS256. The token uses providerUID as the Firebase Auth subject.
func (c *FirebaseClient) mintCustomToken() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": c.serviceAccountEmail,
		"sub": c.serviceAccountEmail,
		"aud": customTokenAudience,
		"iat": now.Unix(),
		"exp": now.Add(tokenLifetime).Unix(),
		"uid": providerUID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(c.privateKey)
}

// parseRSAKey parses a PKCS8 (or PKCS1 fallback) RSA private key from PEM.
// Firebase service account keys use PKCS8 format.
func parseRSAKey(pemStr string) (*rsa.PrivateKey, error) {
	// JSON-encoded keys use literal \n — normalise to actual newlines.
	pemStr = strings.ReplaceAll(pemStr, `\n`, "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found — check service_account_key format")
	}

	// Try PKCS8 first (Firebase service accounts).
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("service_account_key is not an RSA private key")
		}
		return rsaKey, nil
	}

	// Fallback: PKCS1.
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
