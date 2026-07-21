package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// hardcodedDefaults returns the hardcoded default values for a ProjectConfig.
// These match the Python updater-rpc CONF dict.
func hardcodedDefaults() ProjectConfig {
	return ProjectConfig{
		Basic: BasicConfig{
			APIType:     "",
			AccountName: "",
			ProjectName: "",
			PageURL:     "",
			APIURL:      "",
		},
		Download: DownloadConfig{
			Keyword:              nil,
			UpdateKeyword:        nil,
			ExcludeKeyword:       nil,
			Filetype:             StringOrSlice{"7z"},
			Regexes:              nil,
			URL:                  "",
			AddVersionToFilename: false,
			FilenameOverride:     "",
			Path:                 nil,
			Index:                0,
			Indexes:              nil,
			TryRedirect:          true,
			Data:                 nil,
		},
		Version: VersionConfig{
			Path:          nil,
			UseExeVersion: false,
			UseCmdVersion: false,
			FromPage:      false,
			Index:         0,
			Regex:         "",
		},
		Decompress: DecompressConfig{
			Skip:                      BoolOrString{BoolVal: false},
			IncludeFileType:           nil,
			ExcludeFileType:           nil,
			ExcludeFileTypeWhenUpdate: nil,
			SingleDir:                 BoolOrString{BoolVal: true},
			KeepDownloadFile:          true,
			UseBuiltinZipfile:         false,
			UseSystemPackageManager:   false,
			CleanInstall:              false,
		},
		Process: ProcessConfig{
			ImageName:    "",
			AllowRestart: false,
			Service:      false,
			RestartWait:  3,
			StopCmd:      "",
			StartCmd:     "",
			Popup:        true, // match python default behavior
		},
		Build: BuildConfig{
			NoPull: false,
			Branch: "",
		},
	}
}

// Config is the top-level application configuration.
type Config struct {
	Aria2        Aria2Config     `json:"aria2"`
	Binaries     BinariesConfig  `json:"binarys"`
	Requests     RequestsConfig  `json:"requests"`
	Projects     []ProjectEntry  `json:"projects"`
	Repositories []string        `json:"repository,omitzero"`
	Defaults     json.RawMessage `json:"defaults,omitzero"`
	LocalDir     string          `json:"local-dir,omitzero"`
	Metadata     []MetadataRepo  `json:"metadata,omitzero"`
	JSONVer      string          `json:"jsonver,omitzero"`
}

// BinariesConfig configures binary paths.
type BinariesConfig struct {
	Aria2c     string `json:"aria2c"`
	Libarchive string `json:"libarchive,omitzero"`
}

// RequestsConfig configures HTTP request behavior.
type RequestsConfig struct {
	Proxy   string  `json:"proxy,omitzero"`
	Timeout float64 `json:"timeout,omitzero"`
	Retry   int     `json:"retry,omitzero"`
}

func (r RequestsConfig) GetTimeout() time.Duration {
	return time.Duration(r.Timeout) * time.Second
}

// Aria2Config configures the aria2 RPC connection.
type Aria2Config struct {
	IP            string `json:"ip,omitzero"`              // "127.0.0.1" or remote host
	RPCListenPort string `json:"rpc-listen-port,omitzero"` // "6800"
	Schema        string `json:"schema,omitzero"`
	RPCSecret     string `json:"rpc-secret,omitzero"`
	RemoteDir     string `json:"remote-dir,omitzero"` // only when using remote aria2
	LocalDir      string `json:"local-dir,omitzero"`  // only when using remote aria2
}

// RPCAddr returns the full RPC endpoint URL.
func (a Aria2Config) RPCAddr() string {
	return fmt.Sprintf("%s://%s:%s/jsonrpc", a.Schema, a.IP, a.RPCListenPort)
}

// IsRemote returns true when aria2 runs on a different host.
func (a Aria2Config) IsRemote() bool {
	return a.IP != "" && a.IP != "127.0.0.1" && a.IP != "localhost"
}

// MetadataRepo describes a remote metadata repository.
type MetadataRepo struct {
	URL string `json:"url"`
}

// ProjectEntry describes one project in the main config.
type ProjectEntry struct {
	Name     string `json:"name,omitzero"`
	SavePath string `json:"path,omitzero"`
	Version  string `json:"currentVersion,omitzero"`
	Hold     bool   `json:"hold,omitzero"`
}

func (p ProjectEntry) Enabled() bool {
	return !p.Hold
}

// Load reads and parses the main config.json file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{}

	// First, try to detect legacy format (projects as map)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if projRaw, ok := raw["projects"]; ok {
		// Try array first (new format)
		var projects []ProjectEntry
		if err := json.Unmarshal(projRaw, &projects); err == nil {
			cfg.Projects = projects
		} else {
			// Try map (legacy format)
			var projMap map[string]ProjectEntry
			if err := json.Unmarshal(projRaw, &projMap); err != nil {
				return nil, fmt.Errorf("projects field is neither array nor map: %w", err)
			}
			for name, p := range projMap {
				p.Name = name
				cfg.Projects = append(cfg.Projects, p)
			}
		}
		delete(raw, "projects")
	}

	// Unmarshal the rest of the config
	rest, _ := json.Marshal(raw)
	if err := json.Unmarshal(rest, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// applyDefaults fills in sensible defaults for missing config values.
func applyDefaults(cfg *Config) {
	if cfg.Aria2.IP == "" {
		cfg.Aria2.IP = "127.0.0.1"
	}
	if cfg.Aria2.RPCListenPort == "" {
		cfg.Aria2.RPCListenPort = "6800"
	}
	if cfg.Aria2.Schema == "" {
		cfg.Aria2.Schema = "ws"
	}
	if cfg.Requests.Timeout == 0 {
		cfg.Requests.Timeout = 30
	}
	if cfg.Requests.Retry == 0 {
		cfg.Requests.Retry = 5
	}
	if cfg.Binaries.Aria2c == "" {
		cfg.Binaries.Aria2c = "aria2c"
	}
	if cfg.Repositories == nil {
		cfg.Repositories = []string{"https://raw.githubusercontent.com/deorth-kku/updater-config/master"}
	}
}

func GetDefault() *Config {
	cfg := new(Config)
	applyDefaults(cfg)
	return cfg
}

// ProjectConfigPath returns the local override path for a project config.
func ProjectConfigPath(root, name string) string {
	return filepath.Join(root, "config", name+".json")
}

// ApplyDefaults merges the project-level defaults into pc.
// The merge order is: hardcoded defaults → Config.Defaults → file contents.
// This matches the Python pattern of unmarshal-over.
//
// pcFileBytes is the raw JSON bytes from the project config file.
// defaults is the "defaults" field from the main config.json (may be nil).
func ApplyDefaults(pc *ProjectConfig, pcFileBytes, defaults json.RawMessage) error {
	// 1. Start with hardcoded defaults
	base := hardcodedDefaults()

	// 2. Overlay Config.Defaults (project-level defaults from main config)
	if len(defaults) > 0 {
		if err := json.Unmarshal(defaults, &base); err != nil {
			return fmt.Errorf("unmarshal defaults: %w", err)
		}
	}

	// 3. Overlay the actual project config file on top
	if len(pcFileBytes) > 0 {
		if err := json.Unmarshal(pcFileBytes, &base); err != nil {
			return fmt.Errorf("unmarshal project config: %w", err)
		}
	}

	*pc = base
	return nil
}
