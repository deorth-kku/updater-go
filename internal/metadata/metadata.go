// Package metadata manages remote repository metadata for project config discovery.
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deorth-kku/updater-go/internal/api"
)

// Entry maps a project name to its config file path within a repo.
type Entry struct {
	ConfigPath string `json:"config_path"`
	Date       string `json:"date,omitzero"`
	URL        string `json:"url"`
}

// Store holds metadata from all configured repos.
type Store struct {
	mu             sync.RWMutex
	repos          []string
	entries        map[string]Entry
	httpDL         api.Downloader
	localConfigDir string
}

// NewStore creates a new metadata Store.
func NewStore(repos []string, httpDL api.Downloader) *Store {
	return &Store{
		repos:   repos,
		entries: make(map[string]Entry),
		httpDL:  httpDL,
	}
}

// SetLocalConfigDir sets the local directory for storing project configs.
func (s *Store) SetLocalConfigDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localConfigDir = dir
	os.MkdirAll(dir, 0o755)
}

// Load fetches metadata.json from all configured repos and indexes entries.
func (s *Store) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, repoURL := range s.repos {
		metaURL := repoURL
		if metaURL[len(metaURL)-1] != '/' {
			metaURL += "/"
		}
		metaURL += "metadata.json"

		resp, err := s.httpDL.Get(ctx, metaURL, nil)
		if err != nil {
			fmt.Printf("getting metadata from repo %s failed, cause: %s .skipping\n", repoURL, err)
			continue
		}

		var entries map[string]Entry
		if err := json.Unmarshal(resp.Body, &entries); err != nil {
			fmt.Printf("parse metadata from %s failed: %s\n", metaURL, err)
			continue
		}

		for name, entry := range entries {
			// Build full URL: repo_url + config_path
			fullURL := repoURL
			if fullURL[len(fullURL)-1] != '/' {
				fullURL += "/"
			}
			fullURL += entry.ConfigPath
			entry.URL = fullURL
			s.entries[name] = entry
		}
	}

	if len(s.entries) == 0 {
		return fmt.Errorf("cannot get metadata from any repo, please check your repository configuration")
	}

	return nil
}

// GetEntry returns the metadata entry for a project name.
func (s *Store) GetEntry(name string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[name]
	return entry, ok
}

// Entries returns a copy of all metadata entries.
func (s *Store) Entries() map[string]Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Entry, len(s.entries))
	maps.Copy(out, s.entries)
	return out
}

// EnsureLocalConfig checks if a local config exists and is up-to-date.
// If not, downloads the latest config from remote.
func (s *Store) EnsureLocalConfig(ctx context.Context, name string) error {
	s.mu.RLock()
	entry, ok := s.entries[name]
	localDir := s.localConfigDir
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("project %s not found in metadata", name)
	}

	localPath := filepath.Join(localDir, name+".json")

	// Check if local config exists
	if _, err := os.Stat(localPath); err == nil {
		// Local config exists, check if it needs updating
		if entry.Date == "" {
			// No date in metadata, use local config
			return nil
		}

		localInfo, err := os.Stat(localPath)
		if err != nil {
			return fmt.Errorf("stat local config: %w", err)
		}

		remoteDate, err := time.Parse("2006-01-02 15:04:05.000000", entry.Date)
		if err != nil {
			// Can't parse date, use local config
			return nil
		}

		if localInfo.ModTime().Before(remoteDate) {
			// Local config is older, download new version
			fmt.Printf("config file for %s needs downloading\n", name)
			return s.downloadConfig(ctx, entry, localPath)
		}

		// Local config is up-to-date
		return nil
	}

	// Local config doesn't exist, download it
	fmt.Printf("config file for %s not exist, needs downloading\n", name)
	return s.downloadConfig(ctx, entry, localPath)
}

// downloadConfig downloads a config file from remote and saves it locally.
func (s *Store) downloadConfig(ctx context.Context, entry Entry, localPath string) error {
	resp, err := s.httpDL.Get(ctx, entry.URL, nil)
	if err != nil {
		return fmt.Errorf("download config for %s: %w", entry.URL, err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(localPath, resp.Body, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
