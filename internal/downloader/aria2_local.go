// Package downloader coordinates file downloads via aria2 RPC with
// WebSocket callback-based completion detection.
package downloader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"

	aria2 "github.com/deorth-kku/aria2rpc-go"
)

// LocalAria2 manages a local aria2c subprocess as fallback when RPC is unreachable.
type LocalAria2 struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

var ErrNotLocal = errors.New("not a local address")

// StartLocalAria2 starts a local aria2c subprocess if the RPC endpoint is unreachable.
// Returns the local aria2 manager and the (possibly new) secret.
func StartLocalAria2(ctx context.Context, addr, secret, binPath string, logger *slog.Logger) (*LocalAria2, string, error) {
	// Connection failed - check if it's a localhost address
	u, err := url.Parse(addr)
	if err != nil {
		return nil, secret, fmt.Errorf("parse aria2 addr %s: %w", addr, err)
	}

	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "127.1" {
		// Remote aria2 - don't start local subprocess
		return nil, secret, ErrNotLocal
	}

	// Generate a random secret if none provided
	if secret == "" {
		secret = generateSecret()
	}

	// Build aria2c command
	port := u.Port()
	args := []string{
		"--no-conf",
		"--rpc-listen-port=" + port,
		"--enable-rpc=true",
		"--rpc-secret=" + secret,
		"--rpc-listen-all=true",
		"--rpc-allow-origin-all=true",
		"--allow-overwrite=true",
		"--auto-file-renaming=false",
	}

	if binPath == "" {
		binPath = "aria2c"
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	logger.Info("starting local aria2c subprocess", "cmd", binPath, "port", port, "schema", u.Scheme)

	if err := cmd.Start(); err != nil {
		return nil, secret, fmt.Errorf("start aria2c: %w", err)
	}

	// Wait for aria2 to become available (up to 1 second)
	connected := false
	dialer := new(net.Dialer)
	for range 10 {
		time.Sleep(100 * time.Millisecond)
		dialctx, cancel := context.WithTimeout(ctx, 100*time.Second)
		defer cancel()
		conn, err := dialer.DialContext(dialctx, "tcp", net.JoinHostPort(host, port))
		if err == nil {
			connected = true
			conn.Close()
			break
		}
	}

	if !connected {
		cmd.Process.Kill()
		return nil, secret, fmt.Errorf("aria2c failed to start within 1 second")
	}

	logger.Info("local aria2c started successfully", "pid", cmd.Process.Pid)

	// Return a manager that will stop aria2c when the context is cancelled
	localCtx, localCancel := context.WithCancel(ctx)
	la := &LocalAria2{
		cmd:    cmd,
		cancel: localCancel,
	}

	go func() {
		<-localCtx.Done()
		logger.Info("shutting down local aria2c", "pid", cmd.Process.Pid)
		// Send shutdown RPC
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		opts3 := []aria2.Option{}
		if secret != "" {
			opts3 = append(opts3, aria2.WithSecret(secret))
		}
		shutdownClient, err := aria2.New(shutdownCtx, addr, opts3...)
		if err == nil {
			shutdownClient.Shutdown(shutdownCtx)
			shutdownClient.Close()
		}
		// Wait for process to exit
		cmd.Process.Wait()
	}()

	return la, secret, nil
}

// Stop stops the local aria2c subprocess.
func (la *LocalAria2) Stop() {
	if la != nil && la.cancel != nil {
		la.cancel()
	}
}

// generateSecret generates a random hex string for aria2 secret.
func generateSecret() string {
	const chars = "0123456789abcdef"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return strings.ToLower(string(b))
}

func NewAria2DownloaderOrLocal(ctx context.Context, addr, secret, remoteDir, localDir, binPath string, logger *slog.Logger, timeout time.Duration) (*Aria2Downloader, *LocalAria2, error) {
	rpc, err := NewAria2Downloader(ctx, addr, secret, remoteDir, localDir, logger, timeout)
	if err != nil {
		goto try_local
	}
	_, err = rpc.client.GetVersion(ctx)
	if err != nil {
		rpc.Close()
		goto try_local
	}
	return rpc, nil, err
try_local:
	local, newsecret, localerr := StartLocalAria2(ctx, addr, secret, binPath, logger)
	switch localerr {
	case nil:
		rpc, err = NewAria2Downloader(ctx, addr, newsecret, remoteDir, localDir, logger, timeout)
		if err != nil {
			local.Stop()
			return nil, nil, err
		}
		return rpc, local, err
	case ErrNotLocal:
		return nil, nil, err

	default:
		return nil, nil, localerr
	}
}
