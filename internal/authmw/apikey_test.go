package authmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAPIKey(t *testing.T) {
	e := echo.New()
	okHandler := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	mw := APIKey("correct-secret")

	cases := []struct {
		name       string
		headerVal  string
		wantStatus int
	}{
		{"correct key", "correct-secret", http.StatusOK},
		{"wrong key", "wrong-secret", http.StatusUnauthorized},
		{"missing key", "", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/tweet", nil)
			if tc.headerVal != "" {
				req.Header.Set(headerName, tc.headerVal)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := mw(okHandler)(c); err != nil {
				t.Fatalf("middleware returned error: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
