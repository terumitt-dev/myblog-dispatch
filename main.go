package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/terumitt-dev/myblog-dispatch/handler"
	"github.com/terumitt-dev/myblog-dispatch/internal/authmw"
	"github.com/terumitt-dev/myblog-dispatch/internal/distlock"
	"github.com/terumitt-dev/myblog-dispatch/internal/k8sclient"
	"github.com/terumitt-dev/myblog-dispatch/internal/oauthflow"
	"github.com/terumitt-dev/myblog-dispatch/internal/secretsmanager"
	"github.com/terumitt-dev/myblog-dispatch/internal/tokenstore"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
)

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	namespace := getenvDefault("TOKEN_SECRET_NAMESPACE", "go-lilaregard")
	secretName := getenvDefault("TOKEN_SECRET_NAME", "myblog-dispatch-x-tokens")
	leaseName := getenvDefault("TOKEN_REFRESH_LEASE_NAME", "myblog-dispatch-x-refresh-lock")

	k8sClient, err := k8sclient.InCluster()
	if err != nil {
		log.Fatalf("failed to initialize k8s client: %v", err)
	}
	store := tokenstore.New(k8sClient, namespace, secretName)
	// 複数レプリカ構成でも Refresh Token のローテーション競合が起きないよう、
	// リフレッシュ処理を Lease でクラスタ全体に直列化する。
	refreshLock := distlock.New(k8sClient, namespace, leaseName, "")

	// X_CLIENT_ID / X_CLIENT_SECRET は Drone secrets 経由の k8s Secret 注入ではなく、
	// AWS SDK のデフォルト認証チェーン (EC2インスタンスプロファイル) 経由で
	// Secrets Manager から直接取得する。
	oauthSecretID := getenvDefault("X_OAUTH_SECRET_ID", "myblog-dispatch/x-oauth-client")
	creds, err := secretsmanager.FetchXOAuthCredentials(context.Background(), oauthSecretID)
	if err != nil {
		log.Fatalf("failed to fetch X OAuth credentials from Secrets Manager: %v", err)
	}
	xCfg := xauth.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
	}
	manager := xauth.NewManager(store, xCfg, &http.Client{Timeout: 10 * time.Second}, refreshLock)

	tweetHandler := &handler.TweetHandler{
		Manager:    manager,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	dispatchAPIKey := mustGetenv("DISPATCH_API_KEY")
	e.POST("/tweet", tweetHandler.Tweet, authmw.APIKey(dispatchAPIKey))

	// X連携(OAuth Authorization Code + PKCE)のconnect/callbackは、
	// myblog-backendからサーバー間通信でのみ呼ばれる想定 (X-Dispatch-Key必須)。
	// Xからのリダイレクト自体はbackendの公開ドメインで受け、backendがcode/state
	// をここに転送する。state/code_verifierはプロセスメモリでTTL管理し、
	// 使い捨てにする (replicas=1運用が前提、複数レプリカにする場合は要見直し)。
	oauthHandler := &handler.OAuthHandler{
		Config:      xCfg,
		RedirectURI: mustGetenv("X_OAUTH_REDIRECT_URI"),
		States:      oauthflow.NewStore(5 * time.Minute),
		Store:       store,
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
	}
	e.POST("/oauth/connect", oauthHandler.Connect, authmw.APIKey(dispatchAPIKey))
	e.POST("/oauth/callback", oauthHandler.Callback, authmw.APIKey(dispatchAPIKey))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	s := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	e.Logger.Fatal(e.StartServer(s))
}
