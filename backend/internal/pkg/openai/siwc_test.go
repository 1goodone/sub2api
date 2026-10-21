package openai

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestSIWCIdentityRejectsWrongAudienceNonceIssuerAndSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keys, err := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, aud, nonce, issuer string
		key                      *rsa.PrivateKey
		valid                    bool
	}{
		{"valid", "issued", "nonce", "https://auth.openai.com", key, true},
		{"wrong audience", "codex", "nonce", "https://auth.openai.com", key, false},
		{"wrong nonce", "issued", "different", "https://auth.openai.com", key, false},
		{"wrong issuer", "issued", "nonce", "https://example.invalid", key, false},
		{"wrong signature", "issued", "nonce", "https://auth.openai.com", other, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := SIWCIdentity{RegisteredClaims: jwt.RegisteredClaims{Issuer: tc.issuer, Subject: "subscriber", Audience: jwt.ClaimStrings{tc.aud}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Nonce: tc.nonce}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test"
			raw, err := token.SignedString(tc.key)
			require.NoError(t, err)
			identity, err := VerifySIWCIdentity(raw, "issued", "nonce", keys)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "subscriber", identity.Subject)
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestSIWCReturningAuthorizationReusesRegistration(t *testing.T) {
	first, err := url.Parse(BuildSIWCAuthorizationURL("state", "pkce", "nonce", "urn:uuid:host", "", "", "email@example.test"))
	require.NoError(t, err)
	require.Equal(t, SIWCDynamicClientID, first.Query().Get("client_id"))
	require.Equal(t, "Sub2API", first.Query().Get("agent_name_hint"))
	next, err := url.Parse(BuildSIWCAuthorizationURL("new-state", "new-pkce", "new-nonce", "urn:uuid:host", "issued", "saved-id-token", "email@example.test"))
	require.NoError(t, err)
	require.Equal(t, "issued", next.Query().Get("client_id"))
	require.Empty(t, next.Query().Get("agent_name_hint"))
	require.Equal(t, "saved-id-token", next.Query().Get("id_token_hint"))
	require.Equal(t, SIWCRedirectURI, next.Query().Get("redirect_uri"))
}

func TestExtractSIWCCallbackAcceptsFullPasteOrTheURL(t *testing.T) {
	want := "http://127.0.0.1:1455/auth/callback?code=code&state=state&client_id=issued"
	for _, raw := range []string{
		want,
		"  " + want + "  ",
		"无法访问此网站\n" + strings.Replace(want, "127.0.0.1", "localhost", 1) + "\nERR_CONNECTION_REFUSED",
		"\"" + want + "\"",
		"?code=code&state=state&client_id=issued",
		"code=code&state=state&client_id=issued",
		want + "|",
	} {
		got, ok := ExtractSIWCCallback(raw)
		require.True(t, ok, raw)
		require.Equal(t, want, got)
	}
	for _, raw := range []string{
		"https://127.0.0.1:1455/auth/callback?code=code&state=state&client_id=issued",
		"http://evil.example/auth/callback?code=code&state=state&client_id=issued",
		"http://127.0.0.1:1455/other?code=code&state=state&client_id=issued",
		"code-only",
	} {
		_, ok := ExtractSIWCCallback(raw)
		require.False(t, ok, raw)
	}
}
