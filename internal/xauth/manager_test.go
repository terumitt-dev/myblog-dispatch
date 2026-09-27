package xauth

import (
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
