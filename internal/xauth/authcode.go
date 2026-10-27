package xauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authorizeEndpoint は変数として定義し、テストから差し替え可能にしている。
var authorizeEndpoint = "https://twitter.com/i/oauth2/authorize"

// oauthScopes は投稿に必要な最小限のスコープ。offline.access が無いと
// refresh_token が発行されない。
const oauthScopes = "tweet.read tweet.write users.read offline.access"

// CodeChallenge は PKCE の code_verifier から code_challenge (S256) を導出する。
func CodeChallenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// BuildAuthorizeURL は X の認可画面にリダイレクトするための URL を組み立てる。
func BuildAuthorizeURL(cfg Config, redirectURI, state, codeVerifier string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", oauthScopes)
	q.Set("state", state)
	q.Set("code_challenge", CodeChallenge(codeVerifier))
	q.Set("code_challenge_method", "S256")

	return authorizeEndpoint + "?" + q.Encode()
}

// ExchangeAuthorizationCode は Authorization Code フローで取得した code を、
// PKCE の code_verifier とともに Access Token / Refresh Token に交換する。
func ExchangeAuthorizationCode(ctx context.Context, httpClient *http.Client, cfg Config, code, codeVerifier, redirectURI string) (RefreshResult, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)
	form.Set("client_id", cfg.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return RefreshResult{}, fmt.Errorf("build authorization code exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("do authorization code exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("read authorization code exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return RefreshResult{}, fmt.Errorf("authorization code exchange failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed refreshResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return RefreshResult{}, fmt.Errorf("parse authorization code exchange response: %w", err)
	}

	return RefreshResult{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    time.Duration(parsed.ExpiresIn) * time.Second,
	}, nil
}
