package config

import (
	"encoding/json"
	"testing"
)

func TestBoolOrString_MarshalJSON_Bool(t *testing.T) {
	b := BoolOrString{IsBool: true, BoolVal: true}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != "true" {
		t.Errorf("MarshalJSON() = %s, want true", string(data))
	}

	b = BoolOrString{IsBool: true, BoolVal: false}
	data, err = json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != "false" {
		t.Errorf("MarshalJSON() = %s, want false", string(data))
	}
}

func TestBoolOrString_MarshalJSON_String(t *testing.T) {
	b := BoolOrString{IsBool: false, StringVal: "prefix"}
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
	if !b.IsBool || b.BoolVal != true {
		t.Errorf("UnmarshalJSON(true) = {IsBool:%v, BoolVal:%v}, want {true, true}", b.IsBool, b.BoolVal)
	}

	var b2 BoolOrString
	if err := json.Unmarshal([]byte("false"), &b2); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if !b2.IsBool || b2.BoolVal != false {
		t.Errorf("UnmarshalJSON(false) = {IsBool:%v, BoolVal:%v}, want {true, false}", b2.IsBool, b2.BoolVal)
	}
}

func TestBoolOrString_UnmarshalJSON_String(t *testing.T) {
	var b BoolOrString
	if err := json.Unmarshal([]byte(`"single_dir"`), &b); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if b.IsBool || b.StringVal != "single_dir" {
		t.Errorf("UnmarshalJSON(\"single_dir\") = {IsBool:%v, StringVal:%q}, want {false, \"single_dir\"}", b.IsBool, b.StringVal)
	}
}

func TestBoolOrString_Bool(t *testing.T) {
	tests := []struct {
		name string
		b    BoolOrString
		want bool
	}{
		{"bool true", BoolOrString{IsBool: true, BoolVal: true}, true},
		{"bool false", BoolOrString{IsBool: true, BoolVal: false}, false},
		{"string", BoolOrString{IsBool: false, StringVal: "anything"}, true},
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
		{"bool true", BoolOrString{IsBool: true, BoolVal: true}, ""},
		{"bool false", BoolOrString{IsBool: true, BoolVal: false}, ""},
		{"string", BoolOrString{IsBool: false, StringVal: "prefix"}, "prefix"},
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
		s    StringOrSlice
		want string
	}{
		{"single", StringOrSlice{"hello"}, "hello"},
		{"multiple", StringOrSlice{"first", "second"}, "first"},
		{"empty", StringOrSlice{}, ""},
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
