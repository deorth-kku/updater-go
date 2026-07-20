package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Downloader is the interface for fetching remote data (HTML, JSON, RSS).
type Downloader interface {
	// Get performs an HTTP GET and returns the response.
	Get(ctx context.Context, url string) (*HTTPResponse, error)
}

// HTTPResponse represents an HTTP response.
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

// httpClient is the default HTTP downloader backed by net/http.
type httpClient struct {
	client *http.Client
	retry  int // max number of additional attempts on transient failure
}

// NewHTTPClient returns a Downloader with sensible defaults.
func NewHTTPClient(timeout time.Duration) Downloader {
	return NewHTTPClientWithProxy(timeout, "", 0)
}

// NewHTTPClientWithProxy returns a Downloader with the given proxy URL and
// retry count. retry mirrors updater-rpc's HTTPAdapter(max_retries=times):
// transient failures (connection errors, and 429/500/502/503/504 responses)
// are retried up to `retry` times with a short exponential backoff (gap #23).
func NewHTTPClientWithProxy(timeout time.Duration, proxyURL string, retry int) Downloader {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	transport := &http.Transport{}
	if proxyURL != "" {
		transport.Proxy = http.ProxyURL(parseProxyURL(proxyURL))
	}
	return &httpClient{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		retry: retry,
	}
}

// shouldRetry reports whether the given response/error is worth retrying,
// matching requests' default HTTPAdapter retry policy.
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// parseProxyURL normalizes a proxy URL string. If it lacks a scheme, "http://" is prepended.
func parseProxyURL(raw string) *url.URL {
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return u
}

func (h *httpClient) Get(ctx context.Context, url string) (*HTTPResponse, error) {
	var lastErr error
	attempts := h.retry + 1
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 200ms, 400ms, 800ms, ... capped at 2s.
			backoff := 200 * time.Millisecond
			for i := 1; i < attempt; i++ {
				backoff *= 2
			}
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = err
			if !shouldRetry(nil, err) {
				return nil, err
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if !shouldRetry(nil, readErr) {
				return nil, readErr
			}
			continue
		}

		if shouldRetry(resp, nil) {
			lastErr = fmt.Errorf("retryable status %d", resp.StatusCode)
			continue
		}

		return &HTTPResponse{
			StatusCode: resp.StatusCode,
			Body:       body,
			Header:     resp.Header,
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed after %d attempts", attempts)
	}
	return nil, lastErr
}
