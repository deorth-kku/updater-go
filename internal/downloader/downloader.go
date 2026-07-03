// Package downloader coordinates file downloads via aria2 RPC with
// WebSocket callback-based completion detection.
package downloader

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
}

// NewAria2Downloader creates a new aria2 downloader.
func NewAria2Downloader(ctx context.Context, addr, secret, remoteDir, localDir string, logger *slog.Logger, timeout time.Duration) (*Aria2Downloader, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse aria2 addr %s: %w", addr, err)
	}
	useWS := u.Scheme == "ws" || u.Scheme == "wss"

	opts := []aria2.Option{}
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
	done := make(chan struct{}, 1)
	var mu sync.Mutex
	var lastStatus string

	d.client.SetNotificationCallbacks(aria2.NotificationCallbacks{
		OnDownloadComplete: func(ctx context.Context, event aria2.DownloadEvent) {
			if event.GID == gid {
				mu.Lock()
				lastStatus = "complete"
				mu.Unlock()
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
		OnDownloadError: func(ctx context.Context, event aria2.DownloadEvent) {
			if event.GID == gid {
				mu.Lock()
				lastStatus = "error"
				mu.Unlock()
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
		OnDownloadStop: func(ctx context.Context, event aria2.DownloadEvent) {
			if event.GID == gid {
				mu.Lock()
				lastStatus = "stopped"
				mu.Unlock()
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		mu.Lock()
		status := lastStatus
		mu.Unlock()
		if status == "error" {
			statusResp, err := d.client.TellStatus(ctx, gid)
			if err == nil && statusResp != nil {
				return fmt.Errorf("aria2 download error: %s: %s", statusResp.ErrorCode, statusResp.ErrorMessage)
			}
		}
		return nil
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
