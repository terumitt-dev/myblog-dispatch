// Package oauthflow は X OAuth2 Authorization Code + PKCE フローの
// state と code_verifier を、プロセスメモリ上で一時的に保持する。
//
// 値は使い捨て (一度 Consume したら成否に関わらず破棄) かつ TTL 付きで、
// ブラウザや外部ストレージには一切渡らない。myblog-dispatch は
// replicas=1 運用が前提のため、プロセスメモリでの保持で十分としている
// (複数レプリカ運用に変える場合は、distlock と同様に共有ストレージへの
// 移行を検討すること)。
package oauthflow

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	stateByteLength        = 32 // base64url で約43文字
	codeVerifierByteLength = 64 // base64url で約86文字 (PKCE仕様の43〜128文字に収まる)
)

type pendingAuth struct {
	codeVerifier string
	createdAt    time.Time
}

// Store は state をキーに、対応する code_verifier を一時保持する。
type Store struct {
	mu     sync.Mutex
	states map[string]pendingAuth
	ttl    time.Duration
}

// NewStore は ttl で指定した有効期限を持つ Store を構築する。
func NewStore(ttl time.Duration) *Store {
	return &Store{
		states: make(map[string]pendingAuth),
		ttl:    ttl,
	}
}

// New は新しい state と code_verifier のペアを生成し、Store に保持する。
func (s *Store) New() (state, codeVerifier string, err error) {
	state, err = randomURLSafeString(stateByteLength)
	if err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	codeVerifier, err = randomURLSafeString(codeVerifierByteLength)
	if err != nil {
		return "", "", fmt.Errorf("generate code_verifier: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	s.states[state] = pendingAuth{codeVerifier: codeVerifier, createdAt: time.Now()}

	return state, codeVerifier, nil
}

// Consume は state に対応する code_verifier を取得し、Store から削除する。
// 成否に関わらず一度 Consume された state は再利用できない (リプレイ防止)。
func (s *Store) Consume(state string) (codeVerifier string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, exists := s.states[state]
	delete(s.states, state)
	if !exists {
		return "", false
	}
	if time.Since(pending.createdAt) > s.ttl {
		return "", false
	}
	return pending.codeVerifier, true
}

// evictExpiredLocked は期限切れの state を削除する。呼び出し元で mu をロック済みであること。
func (s *Store) evictExpiredLocked() {
	now := time.Now()
	for state, pending := range s.states {
		if now.Sub(pending.createdAt) > s.ttl {
			delete(s.states, state)
		}
	}
}

func randomURLSafeString(byteLength int) (string, error) {
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
