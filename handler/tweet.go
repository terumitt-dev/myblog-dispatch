package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
)

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
