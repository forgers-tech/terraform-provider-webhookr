package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func keyToPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestMintCustomToken(t *testing.T) {
	key := generateTestKey(t)
	c := &FirebaseClient{
		serviceAccountEmail: "sa@project.iam.gserviceaccount.com",
		privateKey:          key,
	}

	token, err := c.mintCustomToken()
	if err != nil {
		t.Fatalf("mintCustomToken: %v", err)
	}

	parsed, err := jwt.Parse(token, func(_ *jwt.Token) (interface{}, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatalf("parsing token: %v", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected MapClaims")
	}

	if got := claims["uid"]; got != providerUID {
		t.Errorf("uid = %v, want %v", got, providerUID)
	}
	if got := claims["iss"]; got != "sa@project.iam.gserviceaccount.com" {
		t.Errorf("iss = %v, want sa@project.iam.gserviceaccount.com", got)
	}
	if got := claims["aud"]; got != customTokenAudience {
		t.Errorf("aud = %v, want %v", got, customTokenAudience)
	}

	exp, _ := claims["exp"].(float64)
	if time.Until(time.Unix(int64(exp), 0)) < 50*time.Minute {
		t.Error("token expires too soon — expected ~1 hour")
	}
}

func TestParseRSAKey_PKCS8(t *testing.T) {
	key := generateTestKey(t)
	pemStr := keyToPEM(t, key)

	parsed, err := parseRSAKey(pemStr)
	if err != nil {
		t.Fatalf("parseRSAKey: %v", err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed key modulus does not match original")
	}
}

func TestParseRSAKey_EscapedNewlines(t *testing.T) {
	key := generateTestKey(t)
	pemStr := keyToPEM(t, key)

	// Simulate a key stored as a JSON string with escaped newlines.
	escaped := strings.ReplaceAll(pemStr, "\n", `\n`)

	parsed, err := parseRSAKey(escaped)
	if err != nil {
		t.Fatalf("parseRSAKey with escaped newlines: %v", err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed key modulus does not match original")
	}
}

func TestParseRSAKey_InvalidPEM(t *testing.T) {
	_, err := parseRSAKey("not a PEM block at all")
	if err == nil {
		t.Error("expected error for invalid input, got nil")
	}
}

func TestParseRSAKey_EmptyString(t *testing.T) {
	_, err := parseRSAKey("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}
