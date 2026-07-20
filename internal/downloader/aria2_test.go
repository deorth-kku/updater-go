package downloader

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deorth-kku/aria2rpc-go"
	"github.com/deorth-kku/updater-go/internal/config"
)

func TestBuildAria2Options(t *testing.T) {
	d := &Aria2Downloader{proxy: "http://proxy:8080", retry: 5}
	opts := d.buildAria2Options("/dl/proj", "file.zip", map[string]string{"Authorization": "Bearer x"})

	if opts["dir"] != "/dl/proj" {
		t.Errorf("dir = %q, want %q", opts["dir"], "/dl/proj")
	}
	if opts["out"] != "file.zip" {
		t.Errorf("out = %q, want %q", opts["out"], "file.zip")
	}
	if opts["split"] != "16" {
		t.Errorf("split = %q, want %q", opts["split"], "16")
	}
	if opts["max-connection-per-server"] != "16" {
		t.Errorf("max-connection-per-server = %q, want %q", opts["max-connection-per-server"], "16")
	}
	if opts["continue"] != "true" {
		t.Errorf("continue = %q, want %q", opts["continue"], "true")
	}
	if opts["proxy"] != "http://proxy:8080" {
		t.Errorf("proxy = %q, want %q", opts["proxy"], "http://proxy:8080")
	}
	if opts["retry"] != "5" {
		t.Errorf("retry = %q, want %q", opts["retry"], "5")
	}
	if opts["header"] != "Authorization: Bearer x" {
		t.Errorf("header = %q, want %q", opts["header"], "Authorization: Bearer x")
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
		Files: []aria2rpc.FileInfo{
			{
				Path: "/mnt/updater-download/proj/file.zip",
			},
		},
	}))
}
