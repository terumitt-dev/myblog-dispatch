package xauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// tweetEndpoint は変数として定義し、テストから差し替え可能にしている。
var tweetEndpoint = "https://api.twitter.com/2/tweets"

type postTweetRequest struct {
	Text string `json:"text"`
}

type postTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// PostTweet は Access Token を使って X にツイートを投稿し、投稿された Tweet ID を返す。
func PostTweet(ctx context.Context, httpClient *http.Client, accessToken, text string) (string, error) {
	payload, err := json.Marshal(postTweetRequest{Text: text})
	if err != nil {
		return "", fmt.Errorf("marshal tweet payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tweetEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build tweet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do tweet request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read tweet response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("post tweet failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed postTweetResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse tweet response: %w", err)
	}

	return parsed.Data.ID, nil
}
