package downloader

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deorth-kku/aria2rpc-go"
	"github.com/deorth-kku/aria2rpc-go/options"
	"github.com/deorth-kku/updater-go/internal/config"
)

func TestBuildAria2Options(t *testing.T) {
	d := &Aria2Downloader{proxy: "http://proxy:8080", retry: 5}
	opts := d.buildAria2Options("/dl/proj", "file.zip", map[string]string{"Authorization": "Bearer x"})

	if opts[options.Dir] != "/dl/proj" {
		t.Errorf("dir = %q, want %q", opts[options.Dir], "/dl/proj")
	}
	if opts[options.Out] != "file.zip" {
		t.Errorf("out = %q, want %q", opts[options.Out], "file.zip")
	}
	if opts[options.Split] != "16" {
		t.Errorf("split = %q, want %q", opts[options.Split], "16")
	}
	if opts[options.MaxConnectionPerServer] != "16" {
		t.Errorf("max-connection-per-server = %q, want %q", opts[options.MaxConnectionPerServer], "16")
	}
	if opts[options.Continue] != "true" {
		t.Errorf("continue = %q, want %q", opts[options.Continue], "true")
	}
	if opts[options.AllProxy] != "http://proxy:8080" {
		t.Errorf("all-proxy = %q, want %q", opts[options.AllProxy], "http://proxy:8080")
	}
	if opts[options.MaxTries] != "5" {
		t.Errorf("max-tries = %q, want %q", opts[options.MaxTries], "5")
	}
	if opts[options.RetryWait] != "1" {
		t.Errorf("retry-wait = %q, want %q", opts[options.RetryWait], "1")
	}
	if opts[options.Header] != "Authorization: Bearer x" {
		t.Errorf("header = %q, want %q", opts[options.Header], "Authorization: Bearer x")
	}
}

func TestResolveLocalPath(t *testing.T) {
	const confstr = `{
        "ip": "aria2.lan",
        "rpc-listen-port": "8080",
        "local-dir": "\\\\download.lan\\mnt\\updater-download",
        "remote-dir": "/mnt/updater-download"
    }`
	var conf config.Aria2Config
	err := json.Unmarshal([]byte(confstr), &conf)
	if err != nil {
		t.Fatal(err)
	}
	rpc := &Aria2Downloader{
		remoteDir: conf.RemoteDir,
		localDir:  conf.LocalDir,
	}
	fmt.Println(rpc.resolveLocalPath(&aria2rpc.Status{
		Files: []aria2rpc.File{
			{
				Path: "/mnt/updater-download/proj/file.zip",
			},
		},
	}))
}
