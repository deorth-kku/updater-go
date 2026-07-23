package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestHTTPClient_RetryOnTransientError verifies gap #23: a request that fails
// (500) a few times is retried and eventually succeeds, up to the configured
// retry count.
func TestHTTPClient_RetryOnTransientError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// retry=5 => up to 6 attempts; first 2 fail, 3rd succeeds.
	c := NewHTTPClientWithProxy(2*time.Second, "", 5)
	resp, err := c.Get(t.Context(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("Body = %q, want %q", string(resp.Body), "ok")
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("server was hit %d times, want 3", atomic.LoadInt32(&hits))
	}
}

// TestHTTPClient_RetryExhausted verifies gap #23: when the server keeps
// returning a retryable status, the client gives up after retry+1 attempts and
// returns an error.
func TestHTTPClient_RetryExhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewHTTPClientWithProxy(2*time.Second, "", 2) // 3 attempts total
	_, err := c.Get(t.Context(), srv.URL, nil)
	if err == nil {
		t.Fatal("Get() should return error after exhausting retries")
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("server was hit %d times, want 3 (retry+1)", atomic.LoadInt32(&hits))
	}
}

// TestHTTPClient_NoRetryOnSuccess verifies the client does not loop when the
// first response is already successful.
func TestHTTPClient_NoRetryOnSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClientWithProxy(2*time.Second, "", 5)
	resp, err := c.Get(t.Context(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("server was hit %d times, want 1", atomic.LoadInt32(&hits))
	}
}
