// Package config defines the project configuration structures.
// The JSON schema is 100% backward-compatible with updater-config/*.json.
package config

import (
	"encoding/json"
	"fmt"
)

// ProjectConfig represents a per-project update configuration.
type ProjectConfig struct {
	Basic       BasicConfig      `json:"basic"`
	Download    DownloadConfig   `json:"download"`
	Version     VersionConfig    `json:"version,omitzero"`
	Decompress  DecompressConfig `json:"decompress,omitzero"`
	Process     ProcessConfig    `json:"process,omitzero"`
	Build       BuildConfig      `json:"build,omitzero"`
	PostCmds    []string         `json:"post-cmds,omitzero"`
	JSONVersion string           `json:"jsonver"`
}

// BasicConfig identifies the API type and the source project.
type BasicConfig struct {
	APIType     string            `json:"api_type"` // "github", "appveyor", "sourceforge", "simplespider", "apijson"
	AccountName string            `json:"account_name,omitzero"`
	ProjectName string            `json:"project_name,omitzero"`
	PageURL     string            `json:"page_url,omitzero"`
	APIURL      string            `json:"api_url,omitzero"`
	Headers     map[string]string `json:"headers,omitzero"`
}

// PathSegment represents a single segment in a JSON path.
// Int >= 0 means array index; Int < 0 means string key stored in Str.
type PathSegment struct {
	Int int    `json:"int,omitzero"`
	Str string `json:"str,omitzero"`
}

// UnmarshalJSON implements custom unmarshaling for PathSegment.
// Accepts either a raw JSON value (number or string) or a structured object.
func (ps *PathSegment) UnmarshalJSON(data []byte) error {
	// Try raw number first
	var num int
	if err := json.Unmarshal(data, &num); err == nil {
		ps.Int = num
		return nil
	}
	// Try raw string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		ps.Str = str
		ps.Int = -1 // Mark as string
		return nil
	}
	// Try structured object
	type pathSegmentAlias PathSegment
	var alias pathSegmentAlias
	if err := json.Unmarshal(data, &alias); err == nil {
		*ps = PathSegment(alias)
		return nil
	}
	return fmt.Errorf("PathSegment must be number, string, or object, got %s", string(data))
}

// IsString returns true if this segment is a string key.
func (ps PathSegment) IsString() bool {
	return ps.Int < 0
}

// StringOrJsonPath is a path element that can be either a literal string
// (e.g. a URL prefix) or a JSON path expression ([]PathSegment).
// If Path is nil, Str holds the literal string value.
type StringOrJsonPath struct {
	Str  string
	Path []PathSegment
}

// UnmarshalJSON implements custom unmarshaling for StringOrJsonPath.
// Accepts either a JSON string or a JSON array of path segments.
func (s *StringOrJsonPath) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Str = str
		return nil
	}
	// Try array of PathSegment
	var path []PathSegment
	if err := json.Unmarshal(data, &path); err == nil {
		s.Path = path
		return nil
	}
	return fmt.Errorf("StringOrJsonPath must be string or []PathSegment, got %s", string(data))
}

type Keywords = Slice[Slice[string]]

func SimpleKeywords(in ...string) Keywords {
	out := make(Keywords, len(in))
	for i, v := range in {
		out[i] = []string{v}
	}
	return out
}

// DownloadConfig controls how the download URL is constructed and what file to pick.
type DownloadConfig struct {
	Keyword              Keywords           `json:"keyword,omitzero"`
	UpdateKeyword        Keywords           `json:"update_keyword,omitzero"`
	ExcludeKeyword       Keywords           `json:"exclude_keyword,omitzero"`
	Filetype             Slice[string]      `json:"filetype,omitzero"` // string or []string
	Regexes              []string           `json:"regexes,omitzero"`
	URL                  string             `json:"url,omitzero"`
	AddVersionToFilename bool               `json:"add_version_to_filename,omitzero"`
	FilenameOverride     string             `json:"filename_override,omitzero"`
	Path                 []StringOrJsonPath `json:"path,omitzero"` // for apijson: URL prefix + JSON path segments
	Index                int                `json:"index,omitzero"`
	Indexes              []int              `json:"indexes,omitzero"`
	TryRedirect          bool               `json:"try_redirect,omitzero"`
	Data                 map[string]any     `json:"data,omitzero"`
}

// VersionConfig controls how the version string is extracted.
type VersionConfig struct {
	Path          []PathSegment `json:"path,omitzero"`
	UseExeVersion bool          `json:"use_exe_version,omitzero"`
	UseCmdVersion bool          `json:"use_cmd_version,omitzero"`
	FromPage      bool          `json:"from_page,omitzero"`
	Index         int           `json:"index,omitzero"`
	Regex         string        `json:"regex,omitzero"`
}

// DecompressConfig controls post-download extraction behavior.
type DecompressConfig struct {
	Skip                      BoolOrString `json:"skip,omitzero"`
	IncludeFileType           []string     `json:"include_file_type,omitzero"`
	ExcludeFileType           []string     `json:"exclude_file_type,omitzero"`
	ExcludeFileTypeWhenUpdate []string     `json:"exclude_file_type_when_update,omitzero"`
	SingleDir                 BoolOrString `json:"single_dir,omitzero"`
	KeepDownloadFile          bool         `json:"keep_download_file,omitzero"`
	UseBuiltinZipfile         bool         `json:"use_builtin_zipfile,omitzero"`
	UseSystemPackageManager   bool         `json:"use_system_package_manager,omitzero"`
	CleanInstall              bool         `json:"clean_install,omitzero"`
}

// ProcessConfig controls process restart behavior.
type ProcessConfig struct {
	ImageName    string `json:"image_name,omitzero"`
	AllowRestart bool   `json:"allow_restart,omitzero"`
	Service      bool   `json:"service,omitzero"`
	RestartWait  int    `json:"restart_wait,omitzero"`
	StopCmd      string `json:"stop_cmd,omitzero"`
	StartCmd     string `json:"start_cmd,omitzero"`
	Popup        bool   `json:"popup,omitzero"` // show Windows popup when waiting for process to stop
}

// BuildConfig controls build/source-fetch behavior.
type BuildConfig struct {
	NoPull bool   `json:"no_pull,omitzero"`
	Branch string `json:"branch,omitzero"`
}

// Slice allows a JSON field to be either a string or an array of strings.
// This matches the Python config where keyword can be "rpcs3" or ["%arch"].
type Slice[T any] []T

// UnmarshalJSON implements custom unmarshaling for StringOrSlice.
func (s *Slice[T]) UnmarshalJSON(data []byte) error {
	// Try array first
	var arr []T
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// Try element
	var str T
	if err := json.Unmarshal(data, &str); err == nil {
		*s = []T{str}
		return nil
	}
	return fmt.Errorf("keyword must be string or []string, got %s", string(data))
}

// First returns the first element, or empty string if empty.
func (s Slice[T]) First() T {
	if len(s) > 0 {
		return s[0]
	}
	var v T
	return v
}

// BoolOrString allows a JSON field to be either a boolean or a string.
// Used for fields like "skip" and "single_dir" which can be true/false or a directory name.
// Zero value (IsString=false) represents a boolean; IsString=true means a string value.
type BoolOrString struct {
	IsString  bool // true if the value was a string
	BoolVal   bool
	StringVal string
}

// UnmarshalJSON implements custom unmarshaling for BoolOrString.
func (b *BoolOrString) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		b.StringVal = str
		b.IsString = true
		return nil
	}
	// Try bool
	var bl bool
	if err := json.Unmarshal(data, &bl); err == nil {
		b.BoolVal = bl
		b.IsString = false
		return nil
	}
	return fmt.Errorf("bool_or_string must be bool or string, got %s", string(data))
}

// Bool returns the value as a bool. If it's a string, returns true.
func (b BoolOrString) Bool() bool {
	if b.IsString {
		return true
	}
	return b.BoolVal
}

// String returns the value as a string. If it's a bool, returns empty string.
func (b BoolOrString) String() string {
	if b.IsString {
		return b.StringVal
	}
	return ""
}

// MarshalJSON implements custom marshaling for BoolOrString.
// Outputs just the boolean or string value, not the struct.
func (b BoolOrString) MarshalJSON() ([]byte, error) {
	if b.IsString {
		return json.Marshal(b.StringVal)
	}
	return json.Marshal(b.BoolVal)
}
