package xauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostTweet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-access-token" {
			t.Errorf("Authorization = %q, want Bearer my-access-token", got)
		}
		var body postTweetRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Text != "hello world" {
			t.Errorf("text = %q, want hello world", body.Text)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]string{"id": "12345", "text": body.Text},
		})
	}))
	defer server.Close()
	tweetEndpoint = server.URL

	id, err := PostTweet(t.Context(), server.Client(), "my-access-token", "hello world")
	if err != nil {
		t.Fatalf("PostTweet returned error: %v", err)
	}
	if id != "12345" {
		t.Fatalf("id = %q, want 12345", id)
	}
}

func TestPostTweet_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"title":"Unauthorized"}`))
	}))
	defer server.Close()
	tweetEndpoint = server.URL

	_, err := PostTweet(t.Context(), server.Client(), "expired-token", "hello")
	if err == nil {
		t.Fatal("expected error for non-201 response, got nil")
	}
}
