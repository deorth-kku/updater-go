// Package downloader coordinates file downloads via aria2 RPC with
// WebSocket callback-based completion detection.
package downloader

//go:generate stringer -type=downloadStatus

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	aria2 "github.com/deorth-kku/aria2rpc-go"
	"github.com/filecoin-project/go-jsonrpc"
)

// Downloader is the interface for file downloads.
type Downloader interface {
	Download(ctx context.Context, url, filename, saveDir string) (localPath, gid string, err error)
	Remove(gid string) error
	Close() error
}

// Aria2Downloader downloads files via aria2 RPC using WebSocket callbacks.
type Aria2Downloader struct {
	client    *aria2.Client
	remoteDir string
	localDir  string
	useWS     bool // true if addr is ws:// or wss://
	sub       *subscriber
}

//go:generate stringer -type=downloadStatus

//go:generate stringer -type=downloadStatus

// downloadStatus represents the final state of a download.
type downloadStatus int

const (
	statusComplete downloadStatus = iota
	statusError
	statusStopped
)

// event is a download completion event.
type event = downloadStatus

// subscriber distributes aria2 download events to subscribers by GID.
type subscriber struct {
	dist map[string]chan event
	mu   sync.RWMutex
}

// newSubscriber creates a new subscriber.
func newSubscriber() *subscriber {
	return &subscriber{
		dist: make(map[string]chan event),
	}
}

// OnDownloadComplete handles aria2 download complete events.
func (s *subscriber) OnDownloadComplete(ctx context.Context, e aria2.DownloadEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ch, ok := s.dist[e.GID]; ok {
		select {
		case ch <- statusComplete:
		default:
		}
	}
}

// OnDownloadError handles aria2 download error events.
func (s *subscriber) OnDownloadError(ctx context.Context, e aria2.DownloadEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ch, ok := s.dist[e.GID]; ok {
		select {
		case ch <- statusError:
		default:
		}
	}
}

// OnDownloadStop handles aria2 download stop events.
func (s *subscriber) OnDownloadStop(ctx context.Context, e aria2.DownloadEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ch, ok := s.dist[e.GID]; ok {
		select {
		case ch <- statusStopped:
		default:
		}
	}
}

// subscribe returns a channel that receives events for the given GID.
func (s *subscriber) subscribe(gid string) chan event {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan event, 1)
	s.dist[gid] = ch
	return ch
}

// NewAria2Downloader creates a new aria2 downloader.
func NewAria2Downloader(ctx context.Context, addr, secret, remoteDir, localDir string, logger *slog.Logger, timeout time.Duration) (*Aria2Downloader, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse aria2 addr %s: %w", addr, err)
	}
	useWS := u.Scheme == "ws" || u.Scheme == "wss"

	sub := newSubscriber()
	opts := []aria2.Option{
		aria2.WithNotificationCallbacks(aria2.NotificationCallbacks{
			OnDownloadComplete: sub.OnDownloadComplete,
			OnDownloadError:    sub.OnDownloadError,
			OnDownloadStop:     sub.OnDownloadStop,
		}),
	}
	if secret != "" {
		opts = append(opts, aria2.WithSecret(secret))
	}
	if timeout > 0 {
		opts = append(opts, aria2.WithJSONRPCOptions(jsonrpc.WithTimeout(timeout)))
	}
	if logger != nil {
		opts = append(opts, aria2.WithJSONRPCOptions(jsonrpc.WithLogger(logger)))
	}

	client, err := aria2.New(ctx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect aria2 at %s: %w", addr, err)
	}

	return &Aria2Downloader{
		client:    client,
		remoteDir: remoteDir,
		localDir:  localDir,
		useWS:     useWS,
		sub:       sub,
	}, nil
}

// Download adds a URI to aria2 and waits for completion.
func (d *Aria2Downloader) Download(ctx context.Context, dlURL, filename, saveDir string) (string, string, error) {
	aria2Dir := d.remoteDir
	if aria2Dir == "" {
		aria2Dir = saveDir
	}

	opts := map[string]string{
		"dir": aria2Dir,
		"out": filename,
	}

	gid, err := d.client.AddURI(ctx, []string{dlURL}, opts, nil)
	if err != nil {
		return "", "", fmt.Errorf("aria2 addURI: %w", err)
	}

	if err := d.waitForCompletion(ctx, gid); err != nil {
		return "", "", err
	}

	localPath := d.resolveLocalPath(aria2Dir, filename)
	return localPath, gid, nil
}

// resolveLocalPath converts the aria2 save path to a local filesystem path.
func (d *Aria2Downloader) resolveLocalPath(aria2Dir, filename string) string {
	if d.localDir != "" {
		rel, _ := filepath.Rel(d.remoteDir, aria2Dir)
		return filepath.Join(d.localDir, rel, filename)
	}
	return filepath.Join(aria2Dir, filename)
}

// waitForCompletion blocks until the download with the given GID completes or errors.
// Uses WebSocket callbacks when available, falls back to polling for HTTP RPC.
func (d *Aria2Downloader) waitForCompletion(ctx context.Context, gid string) error {
	if d.useWS {
		return d.waitForWS(ctx, gid)
	}
	return d.waitForPoll(ctx, gid)
}

// waitForWS uses only WebSocket callbacks — no polling.
func (d *Aria2Downloader) waitForWS(ctx context.Context, gid string) error {
	ch := d.sub.subscribe(gid)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case status := <-ch:
		switch status {
		case statusError, statusStopped:
			statusResp, err := d.client.TellStatus(ctx, gid)
			if err == nil && statusResp != nil {
				return fmt.Errorf("aria2 download %s: %s: %s", status, statusResp.ErrorCode, statusResp.ErrorMessage)
			}
			return fmt.Errorf("aria2 download %s", status)
		case statusComplete:
			return nil
		default:
			return fmt.Errorf("aria2 download %s", status)
		}
	}
}

// waitForPoll uses only TellStatus polling — no WebSocket.
func (d *Aria2Downloader) waitForPoll(ctx context.Context, gid string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			statusResp, err := d.client.TellStatus(ctx, gid)
			if err != nil {
				continue
			}
			if statusResp == nil {
				continue
			}
			switch statusResp.Status {
			case "complete":
				return nil
			case "error":
				return fmt.Errorf("aria2 download error: %s: %s", statusResp.ErrorCode, statusResp.ErrorMessage)
			case "stopped":
				return nil
			}
		case <-timeout:
			return fmt.Errorf("download timeout after 30 minutes for GID %s", gid)
		}
	}
}

// Remove removes a download from aria2.
func (d *Aria2Downloader) Remove(gid string) error {
	_, err := d.client.Remove(context.Background(), gid)
	return err
}

// Close shuts down the aria2 client.
func (d *Aria2Downloader) Close() error {
	if d.client != nil {
		d.client.Close()
	}
	return nil
}

var _ Downloader = (*Aria2Downloader)(nil)

// Ensure strings is used
