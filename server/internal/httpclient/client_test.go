package httpclient

import (
	"context"
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

func TestNewUsesExplicitTimeoutWithoutDeadline(t *testing.T) {
	client := New(context.Background(), 5*time.Second, false)

	if client.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v, want %v", client.Timeout, 5*time.Second)
	}
	if client.Transport != nil {
		t.Fatalf("Transport = %v, want nil", client.Transport)
	}
}

func TestNewUsesEarlierContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := New(ctx, 5*time.Second, false)
	if client.Timeout <= 0 || client.Timeout > 50*time.Millisecond {
		t.Fatalf("Timeout = %v, want within (0, 50ms]", client.Timeout)
	}
}

func TestNewKeepsExplicitTimeoutWhenDeadlineIsLater(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := New(ctx, 100*time.Millisecond, false)
	if client.Timeout != 100*time.Millisecond {
		t.Fatalf("Timeout = %v, want %v", client.Timeout, 100*time.Millisecond)
	}
}

func TestNewEnablesInsecureSkipVerify(t *testing.T) {
	client := New(context.Background(), time.Second, true)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("TLSClientConfig = %+v, want InsecureSkipVerify=true", transport.TLSClientConfig)
	}
}

func TestNewDoesNotReuseSharedTLSConfig(t *testing.T) {
	client1 := New(context.Background(), time.Second, true)
	client2 := New(context.Background(), time.Second, true)

	transport1 := client1.Transport.(*http.Transport)
	transport2 := client2.Transport.(*http.Transport)
	if transport1.TLSClientConfig == transport2.TLSClientConfig {
		t.Fatal("TLSClientConfig 被共享，期望每次创建独立实例")
	}

	transport1.TLSClientConfig.MinVersion = tls.VersionTLS13
	if transport2.TLSClientConfig.MinVersion == tls.VersionTLS13 {
		t.Fatal("修改 client1 的 TLS 配置影响了 client2")
	}
}
