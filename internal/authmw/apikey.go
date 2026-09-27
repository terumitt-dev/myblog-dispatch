// Package authmw は myblog-dispatch の API を呼び出せるクライアントを
// 共有シークレットで制限するミドルウェアを提供する。
package authmw

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
)

const headerName = "X-Dispatch-Key"

// APIKey は X-Dispatch-Key ヘッダーの値を期待値と定数時間比較し、
// 一致しない場合は 401 を返す。
func APIKey(expected string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			got := c.Request().Header.Get(headerName)
			if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}
