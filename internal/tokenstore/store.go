// Package tokenstore は X (Twitter) OAuth2 トークンを k8s Secret に永続化する。
package tokenstore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
	keyExpiresAt    = "expires_at" // unix seconds
)

// Tokens は X OAuth2 のトークンペアと Access Token の有効期限を表す。
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Expired は Access Token が (安全マージンを差し引いて) 期限切れかどうかを返す。
func (t Tokens) Expired() bool {
	const safetyMargin = 60 * time.Second
	return time.Now().Add(safetyMargin).After(t.ExpiresAt)
}

// Store は k8s Secret を介してトークンを読み書きする。
type Store struct {
	client     kubernetes.Interface
	namespace  string
	secretName string
}

// NewForTest はテストコードから fake clientset を注入して Store を構築するためのヘルパー。
func NewForTest(client kubernetes.Interface, namespace, secretName string) *Store {
	return &Store{client: client, namespace: namespace, secretName: secretName}
}

// NewInCluster は Pod 内の ServiceAccount 資格情報を使って Store を構築する。
func NewInCluster(namespace, secretName string) (*Store, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return &Store{client: clientset, namespace: namespace, secretName: secretName}, nil
}

// Get は Secret から現在のトークンを読み込む。
func (s *Store) Get(ctx context.Context) (Tokens, error) {
	secret, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, s.secretName, metav1.GetOptions{})
	if err != nil {
		return Tokens{}, fmt.Errorf("get secret %s/%s: %w", s.namespace, s.secretName, err)
	}

	expiresAtRaw := string(secret.Data[keyExpiresAt])
	expiresAtUnix, err := strconv.ParseInt(expiresAtRaw, 10, 64)
	if err != nil {
		return Tokens{}, fmt.Errorf("parse %s: %w", keyExpiresAt, err)
	}

	return Tokens{
		AccessToken:  string(secret.Data[keyAccessToken]),
		RefreshToken: string(secret.Data[keyRefreshToken]),
		ExpiresAt:    time.Unix(expiresAtUnix, 0),
	}, nil
}

// Save は新しいトークンを Secret に書き戻す。
func (s *Store) Save(ctx context.Context, tokens Tokens) error {
	patch := corev1.Secret{
		Data: map[string][]byte{
			keyAccessToken:  []byte(tokens.AccessToken),
			keyRefreshToken: []byte(tokens.RefreshToken),
			keyExpiresAt:    []byte(strconv.FormatInt(tokens.ExpiresAt.Unix(), 10)),
		},
	}

	secretsClient := s.client.CoreV1().Secrets(s.namespace)
	current, err := secretsClient.Get(ctx, s.secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("secret %s/%s not found: it must be pre-created by ops", s.namespace, s.secretName)
		}
		return fmt.Errorf("get secret before update: %w", err)
	}

	current.Data[keyAccessToken] = patch.Data[keyAccessToken]
	current.Data[keyRefreshToken] = patch.Data[keyRefreshToken]
	current.Data[keyExpiresAt] = patch.Data[keyExpiresAt]

	if _, err := secretsClient.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %s/%s: %w", s.namespace, s.secretName, err)
	}
	return nil
}
