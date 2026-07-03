// Package metadata manages remote repository metadata for project config discovery.
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/deorth-kku/updater-go/internal/api"
)

// Entry maps a project name to its config file path within a repo.
type Entry struct {
	ConfigPath string `json:"config_path"`
	Date       string `json:"date,omitempty"`
}

// Store holds metadata from all configured repos.
type Store struct {
	mu      sync.RWMutex
	repos   []string
	entries map[string]Entry
	httpDL  api.Downloader
}

// NewStore creates a new metadata Store.
func NewStore(repos []string, httpDL api.Downloader) *Store {
	return &Store{
		repos:   repos,
		entries: make(map[string]Entry),
		httpDL:  httpDL,
	}
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

		resp, err := s.httpDL.Get(ctx, metaURL)
		if err != nil {
			return fmt.Errorf("fetch metadata from %s: %w", metaURL, err)
		}

		var entries map[string]Entry
		if err := json.Unmarshal(resp.Body, &entries); err != nil {
			return fmt.Errorf("parse metadata from %s: %w", metaURL, err)
		}

		for name, entry := range entries {
			s.entries[name] = entry
		}
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
