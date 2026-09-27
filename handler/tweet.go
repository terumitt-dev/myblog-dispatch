package handler

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
	"github.com/terumitt-dev/myblog-dispatch/internal/xauth"
)

// X の文字数制限 (280 文字、CJK は重み2、URL は t.co で一律23文字) を
// クライアント側で完全に再現するのは複雑なため、ここでは明らかに
// 巨大な入力を弾くための緩い上限に留める。実際の280文字制限は
// X API 自身がエラーを返した際に BadGateway として扱われる。
const maxTitleRuneCount = 500

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
	if utf8.RuneCountInString(req.Title) > maxTitleRuneCount {
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
