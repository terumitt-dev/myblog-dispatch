package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type TweetRequest struct {
	Title string `json:"title" validate:"required"`
	URL   string `json:"url"   validate:"required"`
}

type TweetResponse struct {
	TweetID string `json:"tweet_id"`
}

// Tweet は X (Twitter) に記事タイトルと URL をツイートする。
// X API v2 + OAuth 1.0a の実装は後続 PR で追加する。
func Tweet(c echo.Context) error {
	var req TweetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Title == "" || req.URL == "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "title and url are required"})
	}

	// TODO: X API v2 でツイートを投稿する
	return c.JSON(http.StatusCreated, TweetResponse{TweetID: "not_implemented"})
}
