package xauth

// SetTokenEndpointForTesting は X OAuth2 のトークンエンドポイントを差し替える。
// 他パッケージのテストから httptest サーバーに向けるためだけに存在し、
// プロダクションコードから呼んではならない。
func SetTokenEndpointForTesting(url string) (restore func()) {
	original := tokenEndpoint
	tokenEndpoint = url
	return func() { tokenEndpoint = original }
}
