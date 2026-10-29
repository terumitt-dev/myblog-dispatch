package xauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCodeChallenge_Deterministic(t *testing.T) {
	a := CodeChallenge("same-verifier")
	b := CodeChallenge("same-verifier")
	if a != b {
		t.Fatalf("CodeChallenge should be deterministic: %q != %q", a, b)
	}
	if CodeChallenge("verifier-a") == CodeChallenge("verifier-b") {
		t.Fatal("different verifiers must not produce the same code_challenge")
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	authorizeEndpoint = "https://example.test/i/oauth2/authorize"

	cfg := Config{ClientID: "client-id", ClientSecret: "client-secret"}
	raw := BuildAuthorizeURL(cfg, "https://example.com/callback", "state-123", "verifier-abc")

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	q := parsed.Query()

	if got := q.Get("response_type"); got != "code" {
		t.Errorf("response_type = %q, want code", got)
	}
	if got := q.Get("client_id"); got != "client-id" {
		t.Errorf("client_id = %q, want client-id", got)
	}
	if got := q.Get("redirect_uri"); got != "https://example.com/callback" {
		t.Errorf("redirect_uri = %q, want https://example.com/callback", got)
	}
	if got := q.Get("state"); got != "state-123" {
		t.Errorf("state = %q, want state-123", got)
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if got := q.Get("code_challenge"); got != CodeChallenge("verifier-abc") {
		t.Errorf("code_challenge = %q, want %q", got, CodeChallenge("verifier-abc"))
	}
	// client_secret はURLに一切含まれてはならない。
	if q.Has("client_secret") {
		t.Error("authorize URL must not contain client_secret")
	}
}

func TestExchangeAuthorizationCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "client-id" || pass != "client-secret" {
			t.Errorf("unexpected basic auth: user=%s pass=%s ok=%v", user, pass, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.FormValue("code"); got != "auth-code-xyz" {
			t.Errorf("code = %q, want auth-code-xyz", got)
		}
		if got := r.FormValue("code_verifier"); got != "verifier-abc" {
			t.Errorf("code_verifier = %q, want verifier-abc", got)
		}
		if got := r.FormValue("redirect_uri"); got != "https://example.com/callback" {
			t.Errorf("redirect_uri = %q, want https://example.com/callback", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
			"token_type":    "bearer",
		})
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	cfg := Config{ClientID: "client-id", ClientSecret: "client-secret"}
	result, err := ExchangeAuthorizationCode(t.Context(), server.Client(), cfg, "auth-code-xyz", "verifier-abc", "https://example.com/callback")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode returned error: %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExchangeAuthorizationCode_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	_, err := ExchangeAuthorizationCode(t.Context(), server.Client(), Config{}, "bad-code", "verifier", "https://example.com/callback")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}
