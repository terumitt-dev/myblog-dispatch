package secretsmanager

import "testing"

func TestParseCredentials(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		raw := []byte(`{"X_CLIENT_ID":"id-123","X_CLIENT_SECRET":"secret-456"}`)
		creds, err := parseCredentials(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.ClientID != "id-123" || creds.ClientSecret != "secret-456" {
			t.Fatalf("unexpected credentials: %+v", creds)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := parseCredentials([]byte("not json")); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("missing client id", func(t *testing.T) {
		raw := []byte(`{"X_CLIENT_SECRET":"secret-456"}`)
		if _, err := parseCredentials(raw); err == nil {
			t.Fatal("expected error for missing X_CLIENT_ID, got nil")
		}
	})

	t.Run("missing client secret", func(t *testing.T) {
		raw := []byte(`{"X_CLIENT_ID":"id-123"}`)
		if _, err := parseCredentials(raw); err == nil {
			t.Fatal("expected error for missing X_CLIENT_SECRET, got nil")
		}
	})

	t.Run("empty object", func(t *testing.T) {
		if _, err := parseCredentials([]byte(`{}`)); err == nil {
			t.Fatal("expected error for empty object, got nil")
		}
	})
}
