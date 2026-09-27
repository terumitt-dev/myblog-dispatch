package xauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/terumitt-dev/myblog-dispatch/internal/distlock"
	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	return tokenstore.New(client, "test-ns", "x-tokens")
}

// newTestStoreAndClient は Store と、同じ fake clientset を共有した状態で
// Lease ロックも構築できるように clientset 自体も返す。
func newTestStoreAndClient(t *testing.T, data map[string][]byte) (*tokenstore.Store, *fake.Clientset) {
	t.Helper()
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "x-tokens", Namespace: "test-ns"},
		Data:       data,
	})
	return tokenstore.New(client, "test-ns", "x-tokens"), client
}

func TestManager_ValidAccessToken_NotExpired(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	store := newTestStore(t, map[string][]byte{
		"access_token":  []byte("still-valid"),
		"refresh_token": []byte("refresh"),
		"expires_at":    []byte(strconv.FormatInt(future, 10)),
	})

	manager := NewManager(store, Config{}, http.DefaultClient, nil)
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

	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, server.Client(), nil)
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

	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, client, nil)
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

// TestManager_ValidAccessToken_BlockedByOtherPodLock は、他 Pod が既に
// リフレッシュ用ロックを保持している場合、ValidAccessToken がリフレッシュを
// 試みずにエラーを返すことを検証する。
func TestManager_ValidAccessToken_BlockedByOtherPodLock(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "should-not-be-used",
			"refresh_token": "should-not-be-used",
			"expires_in":    7200,
		})
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	past := time.Now().Add(-time.Hour).Unix()
	store, client := newTestStoreAndClient(t, map[string][]byte{
		"access_token":  []byte("expired"),
		"refresh_token": []byte("old-refresh"),
		"expires_at":    []byte(strconv.FormatInt(past, 10)),
	})

	// 別 Pod が既にロックを保持している状態を再現する。
	otherPodLock := distlock.New(client, "test-ns", "refresh-lock", "pod-other")
	if err := otherPodLock.Acquire(t.Context()); err != nil {
		t.Fatalf("otherPodLock.Acquire returned error: %v", err)
	}

	myLock := distlock.New(client, "test-ns", "refresh-lock", "pod-me")
	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, server.Client(), myLock)

	_, err := manager.ValidAccessToken(t.Context())
	if !errors.Is(err, distlock.ErrHeld) {
		t.Fatalf("ValidAccessToken error = %v, want wrapping distlock.ErrHeld", err)
	}
	if requestCount != 0 {
		t.Fatalf("refresh HTTP request was made %d times, want 0 (lock should have blocked it)", requestCount)
	}
}

// TestManager_ValidAccessToken_RechecksAfterAcquiringLock は、ロック取得までの
// 間に別 Pod が既にリフレッシュを完了させていた場合、二重にリフレッシュ
// しないことを検証する。fake clientset の Reactor を使い、1 回目の Get
// (ロック取得前) では期限切れトークンを、2 回目の Get (ロック取得後の
// 再読み込み) では別 Pod によって更新済みの新しいトークンを返すことで、
// 「ロック待ちの間に他 Pod がリフレッシュを終えていた」状況を再現する。
func TestManager_ValidAccessToken_RechecksAfterAcquiringLock(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "should-not-be-used",
			"refresh_token": "should-not-be-used",
			"expires_in":    7200,
		})
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	past := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	future := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)

	store, client := newTestStoreAndClient(t, map[string][]byte{
		"access_token":  []byte("expired"),
		"refresh_token": []byte("old-refresh"),
		"expires_at":    []byte(past),
	})

	getCount := 0
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		getCount++
		if getCount == 1 {
			// 1回目 (ロック取得前): 期限切れのまま。
			return false, nil, nil
		}
		// 2回目以降 (ロック取得後の再読み込み): 他 Pod がリフレッシュ済みとして返す。
		return true, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "x-tokens", Namespace: "test-ns"},
			Data: map[string][]byte{
				"access_token":  []byte("already-refreshed-by-other-pod"),
				"refresh_token": []byte("already-refreshed-by-other-pod-refresh"),
				"expires_at":    []byte(future),
			},
		}, nil
	})

	lock := distlock.New(client, "test-ns", "refresh-lock", "pod-me")
	manager := NewManager(store, Config{ClientID: "id", ClientSecret: "secret"}, server.Client(), lock)

	token, err := manager.ValidAccessToken(t.Context())
	if err != nil {
		t.Fatalf("ValidAccessToken returned error: %v", err)
	}
	if token != "already-refreshed-by-other-pod" {
		t.Fatalf("token = %q, want already-refreshed-by-other-pod (should not have refreshed again)", token)
	}
	if requestCount != 0 {
		t.Fatalf("refresh HTTP request was made %d times, want 0 (should have used the already-refreshed token)", requestCount)
	}
}
