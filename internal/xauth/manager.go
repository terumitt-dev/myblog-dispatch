package xauth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
)

// Manager は Access Token の有効性を確認し、必要なら自動でリフレッシュして
// k8s Secret に書き戻す。
type Manager struct {
	mu         sync.Mutex
	store      *tokenstore.Store
	cfg        Config
	httpClient *http.Client
}

// NewManager は Manager を構築する。
func NewManager(store *tokenstore.Store, cfg Config, httpClient *http.Client) *Manager {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Manager{store: store, cfg: cfg, httpClient: httpClient}
}

// ValidAccessToken は有効な Access Token を返す。期限切れの場合は
// Refresh Token を使って自動更新し、新しいトークンを Secret に保存してから返す。
func (m *Manager) ValidAccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tokens, err := m.store.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("load tokens: %w", err)
	}

	if !tokens.Expired() {
		return tokens.AccessToken, nil
	}

	refreshed, err := refreshAccessToken(ctx, m.httpClient, m.cfg, tokens.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh access token: %w", err)
	}

	newTokens := tokenstore.Tokens{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    time.Now().Add(refreshed.ExpiresIn),
	}
	if err := m.store.Save(ctx, newTokens); err != nil {
		return "", fmt.Errorf("save refreshed tokens: %w", err)
	}

	return newTokens.AccessToken, nil
}
