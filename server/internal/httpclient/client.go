package httpclient

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

// New 根据 context 和 TLS 配置创建 HTTP client。
func New(ctx context.Context, timeout time.Duration, insecureSkipVerify bool) *http.Client {
	actualTimeout := timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			actualTimeout = remaining
		}
	}

	client := &http.Client{Timeout: actualTimeout}
	if insecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	return client
}
