// Package config defines the project configuration structures.
// The JSON schema is 100% backward-compatible with updater-config/*.json.
package config

import (
	"encoding/json"
	"fmt"
)

// ProjectConfig represents a per-project update configuration.
type ProjectConfig struct {
	Basic          BasicConfig      `json:"basic"`
	Download       DownloadConfig   `json:"download"`
	Version        VersionConfig    `json:"version,omitempty"`
	Decompress     DecompressConfig `json:"decompress,omitempty"`
	Process        ProcessConfig    `json:"process,omitempty"`
	Build          BuildConfig      `json:"build,omitempty"`
	CurrentVersion string           `json:"current_version,omitempty"`
	JSONVersion    string           `json:"jsonver"`
}

// BasicConfig identifies the API type and the source project.
type BasicConfig struct {
	APIType     string            `json:"api_type"` // "github", "appveyor", "sourceforge", "simplespider", "apijson"
	AccountName string            `json:"account_name,omitempty"`
	ProjectName string            `json:"project_name,omitempty"`
	PageURL     string            `json:"page_url,omitempty"`
	APIURL      string            `json:"api_url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// DownloadConfig controls how the download URL is constructed and what file to pick.
type DownloadConfig struct {
	Keyword              StringOrSlice          `json:"keyword,omitempty"`
	UpdateKeyword        StringOrSlice          `json:"update_keyword,omitempty"`
	ExcludeKeyword       StringOrSlice          `json:"exclude_keyword,omitempty"`
	Filetype             StringOrSlice          `json:"filetype,omitempty"` // string or []string
	Regexes              []string               `json:"regexes,omitempty"`
	URL                  string                 `json:"url,omitempty"`
	AddVersionToFilename bool                   `json:"add_version_to_filename,omitempty"`
	FilenameOverride     string                 `json:"filename_override,omitempty"`
	Path                 []interface{}          `json:"path,omitempty"` // for apijson: base URL + path segments
	Index                int                    `json:"index,omitempty"`
	Indexes              []int                  `json:"indexes,omitempty"`
	TryRedirect          bool                   `json:"try_redirect,omitempty"`
	Data                 map[string]interface{} `json:"data,omitempty"`
}

// VersionConfig controls how the version string is extracted.
type VersionConfig struct {
	Path          []interface{} `json:"path,omitempty"`
	UseExeVersion bool          `json:"use_exe_version,omitempty"`
	UseCmdVersion bool          `json:"use_cmd_version,omitempty"`
	FromPage      bool          `json:"from_page,omitempty"`
	Index         int           `json:"index,omitempty"`
	Regex         string        `json:"regex,omitempty"`
}

// DecompressConfig controls post-download extraction behavior.
type DecompressConfig struct {
	Skip                      BoolOrString `json:"skip,omitempty"`
	IncludeFileType           []string     `json:"include_file_type,omitempty"`
	ExcludeFileType           []string     `json:"exclude_file_type,omitempty"`
	ExcludeFileTypeWhenUpdate []string     `json:"exclude_file_type_when_update,omitempty"`
	SingleDir                 BoolOrString `json:"single_dir,omitempty"`
	KeepDownloadFile          bool         `json:"keep_download_file,omitempty"`
	UseBuiltinZipfile         bool         `json:"use_builtin_zipfile,omitempty"`
	UseSystemPackageManager   bool         `json:"use_system_package_manager,omitempty"`
	CleanInstall              bool         `json:"clean_install,omitempty"`
}

// ProcessConfig controls process restart behavior.
type ProcessConfig struct {
	ImageName    string `json:"image_name,omitempty"`
	AllowRestart bool   `json:"allow_restart,omitempty"`
	Service      bool   `json:"service,omitempty"`
	RestartWait  int    `json:"restart_wait,omitempty"`
	StopCmd      string `json:"stop_cmd,omitempty"`
	StartCmd     string `json:"start_cmd,omitempty"`
}

// BuildConfig controls build/source-fetch behavior.
type BuildConfig struct {
	NoPull bool   `json:"no_pull,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// StringOrSlice allows a JSON field to be either a string or an array of strings.
// This matches the Python config where keyword can be "rpcs3" or ["%arch"].
type StringOrSlice []string

// UnmarshalJSON implements custom unmarshaling for StringOrSlice.
func (s *StringOrSlice) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{str}
		return nil
	}
	// Try array
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	return fmt.Errorf("keyword must be string or []string, got %s", string(data))
}

// First returns the first element, or empty string if empty.
func (s StringOrSlice) First() string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// BoolOrString allows a JSON field to be either a boolean or a string.
// Used for fields like "skip" and "single_dir" which can be true/false or a directory name.
type BoolOrString struct {
	BoolVal   bool
	StringVal string
	IsBool    bool // true if the value was a boolean
}

// UnmarshalJSON implements custom unmarshaling for BoolOrString.
func (b *BoolOrString) UnmarshalJSON(data []byte) error {
	// Try bool first
	var bl bool
	if err := json.Unmarshal(data, &bl); err == nil {
		b.BoolVal = bl
		b.IsBool = true
		return nil
	}
	// Try string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		b.StringVal = str
		b.IsBool = false
		return nil
	}
	return fmt.Errorf("bool_or_string must be bool or string, got %s", string(data))
}

// Bool returns the value as a bool. If it's a string, returns false.
func (b BoolOrString) Bool() bool {
	if b.IsBool {
		return b.BoolVal
	}
	return false
}

// String returns the value as a string. If it's a bool, returns empty string.
func (b BoolOrString) String() string {
	if !b.IsBool {
		return b.StringVal
	}
	return ""
}

// MarshalJSON implements custom marshaling for BoolOrString.
// Outputs just the boolean or string value, not the struct.
func (b BoolOrString) MarshalJSON() ([]byte, error) {
	if b.IsBool {
		return json.Marshal(b.BoolVal)
	}
	return json.Marshal(b.StringVal)
}
