// Package secretsmanager は X OAuth2 クライアント資格情報 (X_CLIENT_ID /
// X_CLIENT_SECRET) を AWS Secrets Manager から取得する。
//
// Drone secrets 経由の k8s Secret 注入ではなく、起動時に AWS SDK の
// デフォルト認証チェーン (EC2 インスタンスプロファイル) 経由で直接取得する
// 設計のため、アプリ側に静的な AWS 認証情報を持たせる必要がない。
package secretsmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// XOAuthCredentials は X アプリの OAuth2 クライアント資格情報。
type XOAuthCredentials struct {
	ClientID     string `json:"X_CLIENT_ID"`
	ClientSecret string `json:"X_CLIENT_SECRET"`
}

// FetchXOAuthCredentials は secretID で指定された Secrets Manager の
// シークレットから X OAuth2 クライアント資格情報を取得する。
// シークレットの値は {"X_CLIENT_ID":"...","X_CLIENT_SECRET":"..."} という
// JSON で、AWS コンソールから手動投入されている前提。
func FetchXOAuthCredentials(ctx context.Context, secretID string) (XOAuthCredentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return XOAuthCredentials{}, fmt.Errorf("load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return XOAuthCredentials{}, fmt.Errorf("get secret value %s: %w", secretID, err)
	}
	if out.SecretString == nil {
		return XOAuthCredentials{}, fmt.Errorf("secret %s has no string value", secretID)
	}

	creds, err := parseCredentials([]byte(*out.SecretString))
	if err != nil {
		return XOAuthCredentials{}, fmt.Errorf("parse secret %s: %w", secretID, err)
	}
	return creds, nil
}

// parseCredentials は Secrets Manager の値 (JSON) を XOAuthCredentials に変換する。
// AWS SDK 呼び出しから切り離すことで、AWS に接続せずテストできるようにしている。
func parseCredentials(raw []byte) (XOAuthCredentials, error) {
	var creds XOAuthCredentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return XOAuthCredentials{}, fmt.Errorf("unmarshal: %w", err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return XOAuthCredentials{}, fmt.Errorf("missing X_CLIENT_ID or X_CLIENT_SECRET")
	}
	return creds, nil
}
