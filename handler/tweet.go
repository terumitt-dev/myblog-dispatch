package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
)

// X の URL は t.co で一律 23 文字に短縮されるため、本文の実質上限は
// 280 文字からその分を差し引いた値になる。タイトルの上限はそれより
// 十分小さい値に安全マージンとして設定する。
const maxTitleLength = 200

type TweetRequest struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type TweetResponse struct {
	TweetID string `json:"tweet_id"`
}

// TweetHandler は X (Twitter) への投稿処理をまとめたハンドラ。
type TweetHandler struct {
	Manager    *xauth.Manager
	HTTPClient *http.Client
}

// Tweet は X (Twitter) に記事タイトルと URL をツイートする。
func (h *TweetHandler) Tweet(c echo.Context) error {
	var req TweetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Title == "" || req.URL == "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "title and url are required"})
	}
	if len(req.Title) > maxTitleLength {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "title is too long"})
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "url must start with http:// or https://"})
	}

	ctx := c.Request().Context()

	accessToken, err := h.Manager.ValidAccessToken(ctx)
	if err != nil {
		c.Logger().Errorf("failed to obtain valid access token: %v", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed to authenticate with X"})
	}

	text := fmt.Sprintf("%s\n%s", req.Title, req.URL)
	tweetID, err := xauth.PostTweet(ctx, h.HTTPClient, accessToken, text)
	if err != nil {
		c.Logger().Errorf("failed to post tweet: %v", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed to post tweet"})
	}

	return c.JSON(http.StatusCreated, TweetResponse{TweetID: tweetID})
}
