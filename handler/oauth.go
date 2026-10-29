package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/terumitt-dev/myblog-dispatch/internal/oauthflow"
	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
)

// OAuthHandler は X OAuth2 Authorization Code + PKCE フローの
// connect (認可URL発行) / callback (トークン交換・保存) を扱う。
//
// どちらのエンドポイントも、X や管理画面のブラウザから直接ではなく、
// myblog-backend からサーバー間通信 (X-Dispatch-Key 付き) で呼ばれる
// 前提。Xからのリダイレクトはbackendの公開ドメインで受け、backendが
// code/state をこの callback に転送する。
type OAuthHandler struct {
	Config      xauth.Config
	RedirectURI string
	States      *oauthflow.Store
	// Manager はトークン保存を自動リフレッシュと同じ排他制御で行うために使う。
	Manager    *xauth.Manager
	HTTPClient *http.Client
}

// ConnectResponse は認可URL発行結果。
type ConnectResponse struct {
	AuthorizeURL string `json:"authorize_url"`
}

// Connect は新しい state / code_verifier を発行し、Xの認可画面URLを返す。
func (h *OAuthHandler) Connect(c echo.Context) error {
	state, codeVerifier, err := h.States.New()
	if err != nil {
		if errors.Is(err, oauthflow.ErrTooManyPending) {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "too many pending connections, please retry later"})
		}
		c.Logger().Errorf("failed to generate oauth state: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start X connection"})
	}

	authorizeURL := xauth.BuildAuthorizeURL(h.Config, h.RedirectURI, state, codeVerifier)
	return c.JSON(http.StatusOK, ConnectResponse{AuthorizeURL: authorizeURL})
}

// CallbackRequest は X からのリダイレクトで受け取った code/state を、
// backend がそのまま転送してきたもの。
type CallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// Callback はXから返ってきたcodeをトークンに交換し、tokenstoreに保存する。
func (h *OAuthHandler) Callback(c echo.Context) error {
	var req CallbackRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Code == "" || req.State == "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "code and state are required"})
	}

	codeVerifier, ok := h.States.Consume(req.State)
	if !ok {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "invalid or expired state"})
	}

	ctx := c.Request().Context()
	result, err := xauth.ExchangeAuthorizationCode(ctx, h.HTTPClient, h.Config, req.Code, codeVerifier, h.RedirectURI)
	if err != nil {
		c.Logger().Errorf("failed to exchange authorization code: %v", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed to authenticate with X"})
	}

	tokens := tokenstore.Tokens{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(result.ExpiresIn),
	}
	if err := h.Manager.StoreConnectedTokens(ctx, tokens); err != nil {
		c.Logger().Errorf("failed to save tokens: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save X tokens"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "connected"})
}
