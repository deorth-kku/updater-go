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
	"strconv"
	"strings"
	"sync"
	"time"

	aria2 "github.com/deorth-kku/aria2rpc-go"
	"github.com/filecoin-project/go-jsonrpc"
)

// Downloader is the interface for file downloads.
type Downloader interface {
	Download(ctx context.Context, url, filename, saveDir string, headers map[string]string) (localPath, gid string, err error)
	Remove(gid string) error
	Close() error
}

// Aria2Downloader downloads files via aria2 RPC using WebSocket callbacks.
type Aria2Downloader struct {
	client    *aria2.Client
	remoteDir string
	localDir  string
	proxy     string // global HTTP proxy (from main config requests.proxy)
	retry     int    // aria2 retry count (from main config requests.retry)
	useWS     bool   // true if addr is ws:// or wss://
	sub       *subscriber
	logger    *slog.Logger
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

// NewAria2Downloader creates a new aria2 downloader. proxy and retry mirror
// updater-rpc's requests.proxy and requests.retry, forwarded to aria2.
func NewAria2Downloader(ctx context.Context, addr, secret, remoteDir, localDir, proxy string, retry int, logger *slog.Logger, timeout time.Duration) (*Aria2Downloader, error) {
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
		proxy:     proxy,
		retry:     retry,
		useWS:     useWS,
		sub:       sub,
		logger:    logger,
	}, nil
}

// buildAria2Options assembles the aria2 AddURI options, mirroring
// updater-rpc's global aria2 args plus the per-project headers (gap #5).
func (d *Aria2Downloader) buildAria2Options(aria2Dir, filename string, headers map[string]string) map[string]string {
	opts := map[string]string{
		"dir":                       aria2Dir,
		"out":                       filename,
		"split":                     "16",
		"max-connection-per-server": "16",
		"continue":                  "true",
	}
	if d.proxy != "" {
		opts["proxy"] = d.proxy
	}
	if d.retry > 0 {
		opts["retry"] = strconv.Itoa(d.retry)
	}
	if len(headers) > 0 {
		var headerList []string
		for k, v := range headers {
			headerList = append(headerList, fmt.Sprintf("%s: %s", k, v))
		}
		// aria2's "header" option is a multi-valued option expressed as a
		// newline-separated string in the options map.
		opts["header"] = strings.Join(headerList, "\n")
	}
	return opts
}

// Download adds a URI to aria2 and waits for completion. headers are
// per-project custom HTTP headers (basic.headers) forwarded to aria2 via the
// "header" option. Proxy, split, max-connection and continue mirror
// updater-rpc's aria2 global options (gap #5).
func (d *Aria2Downloader) Download(ctx context.Context, dlURL, filename, saveDir string, headers map[string]string) (string, string, error) {
	var aria2Dir string
	if d.remoteDir == "" || d.localDir == "" {
		aria2Dir = saveDir
	} else {
		aria2Dir = d.remoteDir + "/" + filepath.Base(saveDir)
	}

	d.logger.Debug("download prepared",
		"url", dlURL,
		"filename", filename,
		"aria2_dir", aria2Dir,
		"reason", "resolved aria2 save dir from remote/local dir config",
		"result", aria2Dir,
	)

	// Build aria2 options. split/max-connection/continue mirror updater-rpc's
	// global aria2 args (split=16, max-connection-per-server=16, continue=true).
	opts := d.buildAria2Options(aria2Dir, filename, headers)

	gid, err := d.client.AddURI(ctx, []string{dlURL}, opts, nil)
	if err != nil {
		d.logger.Error("download add failed",
			"url", dlURL,
			"filename", filename,
			"reason", "aria2 AddURI returned error",
			"result", "error",
		)
		return "", "", fmt.Errorf("aria2 addURI: %w", err)
	}
	d.logger.Info("download started",
		"url", dlURL,
		"filename", filename,
		"gid", gid,
		"reason", "aria2 accepted download, waiting for completion",
		"result", "in progress",
	)
	stat, err := d.waitForCompletion(ctx, gid)
	if err != nil {
		d.logger.Error("download failed",
			"url", dlURL,
			"filename", filename,
			"gid", gid,
			"reason", "waitForCompletion returned error",
			"result", "error",
		)
		return "", "", err
	}

	localPath := d.resolveLocalPath(stat)
	d.logger.Info("download completed",
		"url", dlURL,
		"filename", filename,
		"gid", gid,
		"path", localPath,
		"reason", "aria2 reported completion",
		"result", localPath,
	)
	return localPath, gid, nil
}

// resolveLocalPath converts the aria2 save path to a local filesystem path.
func (d *Aria2Downloader) resolveLocalPath(stat *aria2.Status) string {
	path := stat.Files[0].Path
	if d.localDir == "" || d.remoteDir == "" {
		return path
	}
	return strings.Replace(path, d.remoteDir, d.localDir, 1)
}

// waitForCompletion blocks until the download with the given GID completes or errors.
// Uses WebSocket callbacks when available, falls back to polling for HTTP RPC.
func (d *Aria2Downloader) waitForCompletion(ctx context.Context, gid string) (*aria2.Status, error) {
	if d.useWS {
		return d.waitForWS(ctx, gid)
	}
	return d.waitForPoll(ctx, gid)
}

// waitForWS uses only WebSocket callbacks — no polling.
func (d *Aria2Downloader) waitForWS(ctx context.Context, gid string) (*aria2.Status, error) {
	ch := d.sub.subscribe(gid)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case status := <-ch:
		switch status {
		case statusError, statusStopped:
			statusResp, err := d.client.TellStatus(ctx, gid)
			if err == nil && statusResp != nil {
				return statusResp, fmt.Errorf("aria2 download %s: %s: %s", status, statusResp.ErrorCode, statusResp.ErrorMessage)
			}
			return statusResp, err
		case statusComplete:
			return d.client.TellStatus(ctx, gid)
		default:
			return nil, fmt.Errorf("aria2 download %s", status)
		}
	}
}

// waitForPoll uses only TellStatus polling — no WebSocket.
func (d *Aria2Downloader) waitForPoll(ctx context.Context, gid string) (*aria2.Status, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			statusResp, err := d.client.TellStatus(ctx, gid)
			if err != nil {
				return nil, err
			}
			if statusResp == nil {
				continue
			}
			switch statusResp.Status {
			case "complete":
				return statusResp, nil
			case "error":
				return nil, fmt.Errorf("aria2 download error: %s: %s", statusResp.ErrorCode, statusResp.ErrorMessage)
			case "stopped":
				return nil, fmt.Errorf("aria2 download stopped: %s", gid)
			}
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
