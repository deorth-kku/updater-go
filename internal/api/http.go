package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// NewHTTPClient returns a Downloader with sensible defaults.
func NewHTTPClient(timeout time.Duration) Downloader {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &httpClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (h *httpClient) Get(ctx context.Context, url string) (*HTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Header:     resp.Header,
	}, nil
}

// doRequest performs an HTTP GET and returns an HTTPResponse.
func doRequest(ctx context.Context, url string) (*HTTPResponse, error) {
	return NewHTTPClient(0).Get(ctx, url)
}

// unmarshalJSON decodes JSON from bytes into v.
func unmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// jsonError is a generic JSON API error response.
type jsonError struct {
	Message string `json:"message"`
}

func (e *jsonError) Error() string {
	return fmt.Sprintf("API error: %s", e.Message)
}
