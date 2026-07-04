package downloader

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/deorth-kku/aria2rpc-go"
	"github.com/deorth-kku/updater-go/internal/config"
)

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
