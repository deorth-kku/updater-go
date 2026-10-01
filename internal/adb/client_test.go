package adb

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gadb "github.com/codeskyblue/go-adbkit/adb"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// fakeADBServer implements gadb.Connector backed by net.Pipe, simulating the
// minimal ADB protocol needed to exercise Client.Install.
type fakeADBServer struct {
	features   string
	installOut string
}

// ConnectionContext implements gadb.Connector.
func (f *fakeADBServer) ConnectionContext(_ context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	go f.serve(server)
	return client, nil
}

func (f *fakeADBServer) serve(conn net.Conn) {
	defer conn.Close()
	for {
		cmd, err := readCmd(conn)
		if err != nil {
			return
		}
		switch {
		case cmd == "host:version":
			writeAll(conn, "OKAY000800040029")
		case cmd == "host:features":
			feat := f.features
			if feat == "" {
				feat = "f1"
			}
			writeAll(conn, "OKAY"+fmt.Sprintf("%04x%s", len(feat), feat))
		case strings.HasPrefix(cmd, "host:transport:"):
			writeAll(conn, "OKAY")
		case strings.HasPrefix(cmd, "abb_exec:"):
			writeAll(conn, "OKAY")
			if n, err := parseSize(cmd); err == nil {
				buf := make([]byte, n)
				io.ReadFull(conn, buf)
			}
			writeAll(conn, f.installOut)
			return
		default:
			return
		}
	}
}

func readCmd(conn net.Conn) (string, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return "", err
	}
	n, err := strconv.ParseInt(string(lenBuf[:]), 16, 32)
	if err != nil {
		return "", err
	}
	cmdBuf := make([]byte, n)
	if _, err := io.ReadFull(conn, cmdBuf); err != nil {
		return "", err
	}
	return string(cmdBuf), nil
}

func writeAll(conn net.Conn, s string) {
	io.WriteString(conn, s)
}

func parseSize(cmd string) (int, error) {
	idx := strings.LastIndex(cmd, "-S ")
	if idx < 0 {
		return 0, fmt.Errorf("no size in %q", cmd)
	}
	return strconv.Atoi(cmd[idx+3:])
}

func TestClient_Install_Streaming(t *testing.T) {
	apkPath := filepath.Join(t.TempDir(), "test.apk")
	if err := os.WriteFile(apkPath, []byte("fake-apk-content"), 0o644); err != nil {
		t.Fatalf("write apk: %v", err)
	}

	srv := &fakeADBServer{features: "abb_exec,f1", installOut: "Success"}
	c := &Client{inner: gadb.NewClientWithConnector(srv), logger: testLogger()}

	out, err := c.Install(t.Context(), "testdevice01", apkPath, "-r -d")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !strings.Contains(out, "Success") {
		t.Errorf("Install() output = %q, want contains Success", out)
	}
}

func TestClient_Install_StreamingFailure(t *testing.T) {
	apkPath := filepath.Join(t.TempDir(), "test.apk")
	if err := os.WriteFile(apkPath, []byte("fake-apk-content"), 0o644); err != nil {
		t.Fatalf("write apk: %v", err)
	}

	srv := &fakeADBServer{features: "abb_exec,f1", installOut: "Failure [INSTALL_FAILED_ALREADY_EXISTS]"}
	c := &Client{inner: gadb.NewClientWithConnector(srv), logger: testLogger()}

	if _, err := c.Install(t.Context(), "testdevice01", apkPath, "-r -d"); err == nil {
		t.Fatal("Install() expected error on failure output, got nil")
	}
}

func TestDeviceLock(t *testing.T) {
	c := &Client{logger: testLogger()}
	same1 := c.deviceLock("testdevice01")
	same2 := c.deviceLock("testdevice01")
	if same1 != same2 {
		t.Error("deviceLock() returned different mutexes for the same serial")
	}
	if same1 == c.deviceLock("testdevice02") {
		t.Error("deviceLock() returned the same mutex for different serials")
	}
}

func TestHasAbbExec(t *testing.T) {
	withFeat := &Client{inner: gadb.NewClientWithConnector(&fakeADBServer{features: "abb_exec,f1"}), logger: testLogger()}
	if !withFeat.hasAbbExec() {
		t.Error("hasAbbExec() = false, want true")
	}
	withoutFeat := &Client{inner: gadb.NewClientWithConnector(&fakeADBServer{features: "f1,f2"}), logger: testLogger()}
	if withoutFeat.hasAbbExec() {
		t.Error("hasAbbExec() = true, want false")
	}
}
