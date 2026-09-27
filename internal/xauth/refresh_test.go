package xauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAccessToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "client-id" || pass != "client-secret" {
			t.Errorf("unexpected basic auth: user=%s pass=%s ok=%v", user, pass, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.FormValue("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", got)
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
	result, err := refreshAccessToken(t.Context(), server.Client(), cfg, "old-refresh")
	if err != nil {
		t.Fatalf("refreshAccessToken returned error: %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ExpiresIn.Seconds() != 7200 {
		t.Fatalf("ExpiresIn = %v, want 7200s", result.ExpiresIn)
	}
}

func TestRefreshAccessToken_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	tokenEndpoint = server.URL

	_, err := refreshAccessToken(t.Context(), server.Client(), Config{}, "expired-refresh")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}
