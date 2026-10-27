package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/terumitt-dev/myblog-dispatch/internal/oauthflow"
	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestOAuthHandler(t *testing.T) (*OAuthHandler, *tokenstore.Store) {
	t.Helper()
	client := fake.NewClientset()
	store := tokenstore.New(client, "test-ns", "x-tokens")

	return &OAuthHandler{
		Config:      xauth.Config{ClientID: "client-id", ClientSecret: "client-secret"},
		RedirectURI: "https://example.com/callback",
		States:      oauthflow.NewStore(5 * time.Minute),
		Store:       store,
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}, store
}

func TestOAuthHandler_Connect(t *testing.T) {
	h, _ := newTestOAuthHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/oauth/connect", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Connect(c); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp ConnectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !strings.Contains(resp.AuthorizeURL, "state=") {
		t.Fatalf("authorize_url missing state param: %s", resp.AuthorizeURL)
	}
	if !strings.Contains(resp.AuthorizeURL, "code_challenge=") {
		t.Fatalf("authorize_url missing code_challenge param: %s", resp.AuthorizeURL)
	}
}

func TestOAuthHandler_Callback_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
		})
	}))
	defer server.Close()

	h, store := newTestOAuthHandler(t)
	h.HTTPClient = server.Client()

	restore := xauth.SetTokenEndpointForTesting(server.URL)
	defer restore()

	state, _, err := h.States.New()
	if err != nil {
		t.Fatalf("States.New returned error: %v", err)
	}

	e := echo.New()
	body := strings.NewReader(`{"code":"auth-code-xyz","state":"` + state + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/callback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	saved, err := store.Get(t.Context())
	if err != nil {
		t.Fatalf("Get after Callback returned error: %v", err)
	}
	if saved.AccessToken != "new-access" || saved.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected saved tokens: %+v", saved)
	}
}

func TestOAuthHandler_Callback_InvalidState(t *testing.T) {
	h, _ := newTestOAuthHandler(t)

	e := echo.New()
	body := strings.NewReader(`{"code":"auth-code-xyz","state":"never-issued"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/callback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback returned error: %v", err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestOAuthHandler_Callback_MissingFields(t *testing.T) {
	h, _ := newTestOAuthHandler(t)

	e := echo.New()
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/callback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback returned error: %v", err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestOAuthHandler_Callback_StateIsSingleUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
		})
	}))
	defer server.Close()

	h, _ := newTestOAuthHandler(t)
	h.HTTPClient = server.Client()
	restore := xauth.SetTokenEndpointForTesting(server.URL)
	defer restore()

	state, _, err := h.States.New()
	if err != nil {
		t.Fatalf("States.New returned error: %v", err)
	}

	e := echo.New()
	makeReq := func() *httptest.ResponseRecorder {
		body := strings.NewReader(`{"code":"auth-code-xyz","state":"` + state + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/oauth/callback", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := h.Callback(c); err != nil {
			t.Fatalf("Callback returned error: %v", err)
		}
		return rec
	}

	first := makeReq()
	if first.Code != http.StatusOK {
		t.Fatalf("first callback status = %d, want 200, body=%s", first.Code, first.Body.String())
	}

	second := makeReq()
	if second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second callback (replay) status = %d, want 422", second.Code)
	}
}
