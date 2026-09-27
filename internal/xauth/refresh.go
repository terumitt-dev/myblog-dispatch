// Package xauth は X (Twitter) API v2 の OAuth2 認証とツイート投稿を扱う。
package xauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tokenEndpoint は変数として定義し、テストから差し替え可能にしている。
var tokenEndpoint = "https://api.twitter.com/2/oauth2/token"

// Config は X アプリの OAuth2 クライアント資格情報。
type Config struct {
	ClientID     string
	ClientSecret string
}

// RefreshResult は Refresh Token を使って取得した新しいトークンセット。
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// refreshAccessToken は Refresh Token を使って新しい Access Token / Refresh Token を取得する。
// X アプリは confidential client (Web App, Automated App or Bot) として設定されている必要がある。
func refreshAccessToken(ctx context.Context, httpClient *http.Client, cfg Config, refreshToken string) (RefreshResult, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", cfg.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return RefreshResult{}, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("do refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return RefreshResult{}, fmt.Errorf("refresh token failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed refreshResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return RefreshResult{}, fmt.Errorf("parse refresh response: %w", err)
	}

	return RefreshResult{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    time.Duration(parsed.ExpiresIn) * time.Second,
	}, nil
}
