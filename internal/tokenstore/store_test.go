package tokenstore

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeStore(t *testing.T, initial map[string][]byte) *Store {
	t.Helper()
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "x-tokens", Namespace: "test-ns"},
		Data:       initial,
	})
	return &Store{client: client, namespace: "test-ns", secretName: "x-tokens"}
}

func TestStore_GetAndSave(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := newFakeStore(t, map[string][]byte{
		keyAccessToken:  []byte("old-access"),
		keyRefreshToken: []byte("old-refresh"),
		keyExpiresAt:    []byte("1700000000"),
	})

	got, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.AccessToken != "old-access" || got.RefreshToken != "old-refresh" {
		t.Fatalf("unexpected tokens: %+v", got)
	}

	newTokens := Tokens{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    now.Add(2 * time.Hour),
	}
	if err := store.Save(context.Background(), newTokens); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err = store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after Save returned error: %v", err)
	}
	if got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" {
		t.Fatalf("tokens not updated: %+v", got)
	}
	if !got.ExpiresAt.Equal(newTokens.ExpiresAt) {
		t.Fatalf("expected ExpiresAt %v, got %v", newTokens.ExpiresAt, got.ExpiresAt)
	}
}

func TestTokens_Expired(t *testing.T) {
	cases := []struct {
		name    string
		expires time.Time
		want    bool
	}{
		{"far future", time.Now().Add(time.Hour), false},
		{"already past", time.Now().Add(-time.Minute), true},
		{"within safety margin", time.Now().Add(30 * time.Second), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := Tokens{ExpiresAt: tc.expires}
			if got := tokens.Expired(); got != tc.want {
				t.Errorf("Expired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStore_Save_NilData(t *testing.T) {
	// Secret に data: フィールドが存在しない (nil map) 状態を再現する。
	store := newFakeStore(t, nil)

	err := store.Save(context.Background(), Tokens{
		AccessToken:  "a",
		RefreshToken: "b",
		ExpiresAt:    time.Now(),
	})
	if err != nil {
		t.Fatalf("Save with nil Data returned error: %v", err)
	}
}

func TestStore_Save_SecretNotFound(t *testing.T) {
	client := fake.NewClientset()
	store := &Store{client: client, namespace: "test-ns", secretName: "missing"}

	err := store.Save(context.Background(), Tokens{AccessToken: "a", RefreshToken: "b", ExpiresAt: time.Now()})
	if err == nil {
		t.Fatal("expected error when secret does not exist, got nil")
	}
}
