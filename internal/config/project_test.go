package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// gamingConfigDir is the path to the gaming configs in updater-config.
var gamingConfigDir = filepath.Join("..", "..", "..", "updater-config", "gaming")

func TestStringOrSlice_String(t *testing.T) {
	var s Slice[string]
	if err := json.Unmarshal([]byte(`"rpcs3"`), &s); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if len(s) != 1 || s[0] != "rpcs3" {
		t.Errorf("StringOrSlice = %v, want [rpcs3]", s)
	}
}

func TestStringOrSlice_Slice(t *testing.T) {
	var s Slice[string]
	if err := json.Unmarshal([]byte(`["%arch", "amd64"]`), &s); err != nil {
		t.Fatalf("unmarshal Slice[string]: %v", err)
	}
	if len(s) != 2 || s[0] != "%arch" || s[1] != "amd64" {
		t.Errorf("StringOrSlice = %v, want [%q, %q]", s, "%arch", "amd64")
	}
}

func TestStringOrSlice_EmptyString(t *testing.T) {
	var s Slice[string]
	if err := json.Unmarshal([]byte(`""`), &s); err != nil {
		t.Fatalf("unmarshal empty string: %v", err)
	}
	if len(s) != 1 || s[0] != "" {
		t.Errorf("StringOrSlice = %v, want [\"\"]", s)
	}
}

func TestStringOrSlice_EmptyArray(t *testing.T) {
	var s Slice[string]
	if err := json.Unmarshal([]byte(`[]`), &s); err != nil {
		t.Fatalf("unmarshal empty array: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("StringOrSlice = %v, want []", s)
	}
}

func TestStringOrSlice_Invalid(t *testing.T) {
	var s Slice[string]
	err := json.Unmarshal([]byte(`123`), &s)
	if err == nil {
		t.Error("unmarshal number: expected error")
	}
}

// TestProjectConfig_Unmarshal_RealFiles loads every JSON from updater-config/gaming/
// and verifies it unmarshals into ProjectConfig without error.
func TestProjectConfig_Unmarshal_RealFiles(t *testing.T) {
	entries, err := os.ReadDir(gamingConfigDir)
	if err != nil {
		t.Fatalf("read updater-config/gaming: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(gamingConfigDir, entry.Name()))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			var pc ProjectConfig
			if err := json.Unmarshal(data, &pc); err != nil {
				t.Fatalf("unmarshal %s: %v", entry.Name(), err)
			}
			// Verify basic fields are populated
			if pc.Basic.APIType == "" {
				t.Error("APIType is empty")
			}
		})
	}
}

// TestProjectConfig_Unmarshal_AllSubdirs loads JSON from all subdirectories.
func TestProjectConfig_Unmarshal_AllSubdirs(t *testing.T) {
	baseDir := filepath.Join("..", "..", "..", "updater-config")
	subdirs := []string{"gaming", "android", "dev", "multimedia", "system-tools", "utils", "windows-tools"}

	for _, sub := range subdirs {
		dir := filepath.Join(baseDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // skip missing dirs
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			t.Run(sub+"/"+entry.Name(), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatalf("read file: %v", err)
				}
				var pc ProjectConfig
				if err := json.Unmarshal(data, &pc); err != nil {
					t.Fatalf("unmarshal %s: %v", entry.Name(), err)
				}
			})
		}
	}
}

// TestProjectConfig_GetProjectConfig verifies that GetProjectConfig fills in missing fields
// using hardcoded defaults and Config.Defaults.
func TestProjectConfig_GetProjectConfig(t *testing.T) {
	// Minimal config — only basic.api_type set
	data := []byte(`{"basic": {"api_type": "github", "account_name": "test", "project_name": "test"}}`)
	pc, err := GetProjectConfig(data, nil)
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}

	// TryRedirect should default to true
	if !pc.Download.TryRedirect {
		t.Error("TryRedirect should default to true")
	}
	// Decompress.Skip should default to false
	if pc.Decompress.Skip.Bool() {
		t.Error("Decompress.Skip should default to false")
	}
	// Process.AllowRestart should default to false
	if pc.Process.AllowRestart {
		t.Error("AllowRestart should default to false")
	}
	// Process.RestartWait should default to 3
	if pc.Process.RestartWait != 3 {
		t.Errorf("RestartWait = %d, want 3", pc.Process.RestartWait)
	}
}

// TestProjectConfig_GetProjectConfig_WithDefaults verifies that Config.Defaults overlay works.
func TestProjectConfig_GetProjectConfig_WithDefaults(t *testing.T) {
	data := []byte(`{"basic": {"api_type": "github", "account_name": "test", "project_name": "test"}}`)
	defaults := json.RawMessage(`{"download": {"try_redirect": false}, "process": {"restart_wait": 5}}`)

	pc, err := GetProjectConfig(data, defaults)
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}

	if pc.Download.TryRedirect {
		t.Error("TryRedirect should be false (overridden by defaults)")
	}
	if pc.Process.RestartWait != 5 {
		t.Errorf("RestartWait = %d, want 5 (overridden by defaults)", pc.Process.RestartWait)
	}
}
func TestBoolOrString_MarshalJSON_Bool(t *testing.T) {
	b := BoolOrString{BoolVal: true}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != "true" {
		t.Errorf("MarshalJSON() = %s, want true", string(data))
	}

	b = BoolOrString{BoolVal: false}
	data, err = json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != "false" {
		t.Errorf("MarshalJSON() = %s, want false", string(data))
	}
}

func TestBoolOrString_MarshalJSON_String(t *testing.T) {
	b := BoolOrString{IsString: true, StringVal: "prefix"}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != `"prefix"` {
		t.Errorf("MarshalJSON() = %s, want \"prefix\"", string(data))
	}
}

func TestBoolOrString_UnmarshalJSON_Bool(t *testing.T) {
	var b BoolOrString
	if err := json.Unmarshal([]byte("true"), &b); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if b.IsString || b.BoolVal != true {
		t.Errorf("UnmarshalJSON(true) = {IsString:%v, BoolVal:%v}, want {false, true}", b.IsString, b.BoolVal)
	}

	var b2 BoolOrString
	if err := json.Unmarshal([]byte("false"), &b2); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if b2.IsString || b2.BoolVal != false {
		t.Errorf("UnmarshalJSON(false) = {IsString:%v, BoolVal:%v}, want {false, false}", b2.IsString, b2.BoolVal)
	}
}

func TestBoolOrString_UnmarshalJSON_String(t *testing.T) {
	var b BoolOrString
	if err := json.Unmarshal([]byte(`"single_dir"`), &b); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if !b.IsString || b.StringVal != "single_dir" {
		t.Errorf("UnmarshalJSON(\"single_dir\") = {IsString:%v, StringVal:%q}, want {true, \"single_dir\"}", b.IsString, b.StringVal)
	}
}

func TestBoolOrString_Bool(t *testing.T) {
	tests := []struct {
		name string
		b    BoolOrString
		want bool
	}{
		{"bool true", BoolOrString{BoolVal: true}, true},
		{"bool false", BoolOrString{BoolVal: false}, false},
		{"string", BoolOrString{IsString: true, StringVal: "anything"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.b.Bool(); got != tt.want {
				t.Errorf("Bool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBoolOrString_String(t *testing.T) {
	tests := []struct {
		name string
		b    BoolOrString
		want string
	}{
		{"bool true", BoolOrString{BoolVal: true}, ""},
		{"bool false", BoolOrString{BoolVal: false}, ""},
		{"string", BoolOrString{IsString: true, StringVal: "prefix"}, "prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.b.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringOrSlice_First(t *testing.T) {
	tests := []struct {
		name string
		s    Slice[string]
		want string
	}{
		{"single", Slice[string]{"hello"}, "hello"},
		{"multiple", Slice[string]{"first", "second"}, "first"},
		{"empty", Slice[string]{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.First(); got != tt.want {
				t.Errorf("First() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathSegment_IsString(t *testing.T) {
	tests := []struct {
		name string
		ps   PathSegment
		want bool
	}{
		{"string key", PathSegment{Str: "key", Int: -1}, true},
		{"array index", PathSegment{Int: 0}, false},
		{"negative index", PathSegment{Int: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ps.IsString(); got != tt.want {
				t.Errorf("IsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringOrJsonPath_UnmarshalJSON_String(t *testing.T) {
	var s StringOrJsonPath
	if err := json.Unmarshal([]byte(`"https://example.com"`), &s); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if s.Str != "https://example.com" {
		t.Errorf("Str = %q, want %q", s.Str, "https://example.com")
	}
	if s.Path != nil {
		t.Errorf("Path = %v, want nil", s.Path)
	}
}

func TestStringOrJsonPath_UnmarshalJSON_Array(t *testing.T) {
	var s StringOrJsonPath
	if err := json.Unmarshal([]byte(`[{"int": 0}, {"str": "key"}]`), &s); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if s.Str != "" {
		t.Errorf("Str = %q, want empty", s.Str)
	}
	if len(s.Path) != 2 {
		t.Fatalf("len(Path) = %d, want 2", len(s.Path))
	}
	if s.Path[0].Int != 0 {
		t.Errorf("Path[0].Int = %d, want 0", s.Path[0].Int)
	}
	if s.Path[1].Str != "key" {
		t.Errorf("Path[1].Str = %q, want %q", s.Path[1].Str, "key")
	}
}

func TestStringOrJsonPath_UnmarshalJSON_Invalid(t *testing.T) {
	var s StringOrJsonPath
	err := json.Unmarshal([]byte(`123`), &s)
	if err == nil {
		t.Error("UnmarshalJSON(123) should return error")
	}
}

func TestPathSegment_UnmarshalJSON_Number(t *testing.T) {
	var ps PathSegment
	if err := json.Unmarshal([]byte(`42`), &ps); err != nil {
		t.Fatalf("UnmarshalJSON(42) error = %v", err)
	}
	if ps.Int != 42 || ps.Str != "" {
		t.Errorf("PathSegment = {Int:%d, Str:%q}, want {Int:42, Str:\"\"}", ps.Int, ps.Str)
	}
}

func TestPathSegment_UnmarshalJSON_String(t *testing.T) {
	var ps PathSegment
	if err := json.Unmarshal([]byte(`"key"`), &ps); err != nil {
		t.Fatalf("UnmarshalJSON(\"key\") error = %v", err)
	}
	if ps.Int != -1 || ps.Str != "key" {
		t.Errorf("PathSegment = {Int:%d, Str:%q}, want {Int:-1, Str:\"key\"}", ps.Int, ps.Str)
	}
}

func TestPathSegment_UnmarshalJSON_Object(t *testing.T) {
	var ps PathSegment
	if err := json.Unmarshal([]byte(`{"int": 5}`), &ps); err != nil {
		t.Fatalf("UnmarshalJSON({int:5}) error = %v", err)
	}
	if ps.Int != 5 {
		t.Errorf("PathSegment.Int = %d, want 5", ps.Int)
	}
}

func TestPathSegment_UnmarshalJSON_Invalid(t *testing.T) {
	var ps PathSegment
	err := json.Unmarshal([]byte(`null`), &ps)
	// null unmarshals to zero value without error
	if err != nil {
		t.Errorf("UnmarshalJSON(null) should not return error, got %v", err)
	}
}

func TestAria2Config_RPCAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  Aria2Config
		want string
	}{
		{name: "http", cfg: Aria2Config{IP: "127.0.0.1", RPCListenPort: "6800", Schema: "http"}, want: "http://127.0.0.1:6800/jsonrpc"},
		{name: "ws", cfg: Aria2Config{IP: "192.168.1.100", RPCListenPort: "6800", Schema: "ws"}, want: "ws://192.168.1.100:6800/jsonrpc"},
		{name: "localhost", cfg: Aria2Config{IP: "localhost", RPCListenPort: "8080", Schema: "http"}, want: "http://localhost:8080/jsonrpc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.RPCAddr(); got != tt.want {
				t.Errorf("RPCAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAria2Config_IsRemote(t *testing.T) {
	tests := []struct {
		name string
		cfg  Aria2Config
		want bool
	}{
		{name: "localhost", cfg: Aria2Config{IP: "127.0.0.1"}, want: false},
		{name: "localhost string", cfg: Aria2Config{IP: "localhost"}, want: false},
		{name: "remote", cfg: Aria2Config{IP: "192.168.1.100"}, want: true},
		{name: "empty", cfg: Aria2Config{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsRemote(); got != tt.want {
				t.Errorf("IsRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectEntry_Enabled(t *testing.T) {
	tests := []struct {
		name  string
		entry ProjectEntry
		want  bool
	}{
		{name: "enabled", entry: ProjectEntry{Hold: false}, want: true},
		{name: "disabled", entry: ProjectEntry{Hold: true}, want: false},
		{name: "default", entry: ProjectEntry{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.Enabled(); got != tt.want {
				t.Errorf("Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestsConfig_GetTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  RequestsConfig
		want int
	}{
		{name: "30s", cfg: RequestsConfig{Timeout: 30}, want: 30},
		{name: "0s", cfg: RequestsConfig{Timeout: 0}, want: 0},
		{name: "60s", cfg: RequestsConfig{Timeout: 60}, want: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetTimeout()
			if int(got.Seconds()) != tt.want {
				t.Errorf("GetTimeout() = %v, want %ds", got, tt.want)
			}
		})
	}
}

func TestGetDefault(t *testing.T) {
	cfg := GetDefault()
	if cfg.Aria2.IP != "127.0.0.1" {
		t.Errorf("Aria2.IP = %q, want %q", cfg.Aria2.IP, "127.0.0.1")
	}
	if cfg.Aria2.RPCListenPort != "6800" {
		t.Errorf("Aria2.RPCListenPort = %q, want %q", cfg.Aria2.RPCListenPort, "6800")
	}
	if cfg.Requests.Timeout != 30 {
		t.Errorf("Requests.Timeout = %v, want 30", cfg.Requests.Timeout)
	}
	if cfg.Requests.Retry != 5 {
		t.Errorf("Requests.Retry = %v, want 5", cfg.Requests.Retry)
	}
}
