package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// cancelAfterRoundTripper は HTTP レスポンスを受け取った直後に呼び出し元の
// context をキャンセルする。「リフレッシュには成功したが、その直後に
// クライアントが切断した」状況を再現するためのテスト用ラウンドトリッパー。
type cancelAfterRoundTripper struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (t *cancelAfterRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if t.cancel != nil {
		t.cancel()
	}
	return resp, err
}

func newTestStore(t *testing.T, data map[string][]byte) *tokenstore.Store {
	t.Helper()
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "x-tokens", Namespace: "test-ns"},
		Data:       data,
	})
	return tokenstore.NewForTest(client, "test-ns", "x-tokens")
}

func TestManager_ValidAccessToken_NotExpired(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	store := newTestStore(t, map[string][]byte{
		"access_token":  []byte("still-valid"),
		"refresh_token": []byte("refresh"),
		"expires_at":    []byte(strconv.FormatInt(future, 10)),
	})

	manager := NewManager(store, Config{}, http.DefaultClient)
	token, err := manager.ValidAccessToken(t.Context())
	if err != nil {
		t.Fatalf("ValidAccessToken returned error: %v", err)
	}
	if token != "still-valid" {
		t.Fatalf("token = %q, want still-valid", token)
	}
}

func TestManager_ValidAccessToken_RefreshesWhenExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed-access",
			"refresh_token": "refreshed-refresh",
			"expires_in":    7200,
		})
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	past := time.Now().Add(-time.Hour).Unix()
	store := newTestStore(t, map[string][]byte{
		"access_token":  []byte("expired"),
		"refresh_token": []byte("old-refresh"),
		"expires_at":    []byte(strconv.FormatInt(past, 10)),
	})

	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, server.Client())
	token, err := manager.ValidAccessToken(t.Context())
	if err != nil {
		t.Fatalf("ValidAccessToken returned error: %v", err)
	}
	if token != "refreshed-access" {
		t.Fatalf("token = %q, want refreshed-access", token)
	}

	saved, err := store.Get(t.Context())
	if err != nil {
		t.Fatalf("Get after refresh returned error: %v", err)
	}
	if saved.RefreshToken != "refreshed-refresh" {
		t.Fatalf("stored refresh token = %q, want refreshed-refresh", saved.RefreshToken)
	}
}

// TestManager_ValidAccessToken_SavesEvenIfRequestContextCancelledAfterRefresh は、
// リフレッシュ成功直後に呼び出し元のリクエストがキャンセルされても、
// 新しいトークンの保存が失敗しないことを検証する。X 側では既に
// Refresh Token がローテーション済みのため、ここで保存に失敗すると
// 復旧不能になる。
func TestManager_ValidAccessToken_SavesEvenIfRequestContextCancelledAfterRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed-access",
			"refresh_token": "refreshed-refresh",
			"expires_in":    7200,
		})
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	past := time.Now().Add(-time.Hour).Unix()
	store := newTestStore(t, map[string][]byte{
		"access_token":  []byte("expired"),
		"refresh_token": []byte("old-refresh"),
		"expires_at":    []byte(strconv.FormatInt(past, 10)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: &cancelAfterRoundTripper{base: server.Client().Transport, cancel: cancel}}

	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, client)
	token, err := manager.ValidAccessToken(ctx)
	if err != nil {
		t.Fatalf("ValidAccessToken returned error even though save should be independent of request ctx: %v", err)
	}
	if token != "refreshed-access" {
		t.Fatalf("token = %q, want refreshed-access", token)
	}

	saved, err := store.Get(t.Context())
	if err != nil {
		t.Fatalf("Get after refresh returned error: %v", err)
	}
	if saved.RefreshToken != "refreshed-refresh" {
		t.Fatalf("stored refresh token = %q, want refreshed-refresh (token was lost when request ctx was cancelled)", saved.RefreshToken)
	}
}
