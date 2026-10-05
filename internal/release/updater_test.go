package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func compressed(t *testing.T, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := gzip.NewWriter(&buffer)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestDownload(t *testing.T) {
	name := "azssh_v1.2.3_linux_amd64.gz"
	good := compressed(t, []byte("verified binary"))
	for _, tc := range []struct {
		name     string
		asset    []byte
		manifest string
		status   int
		want     string
	}{
		{"success", good, fmt.Sprintf("%x  %s\n", sha256.Sum256(good), name), 200, ""},
		{"mismatch", good, fmt.Sprintf("%064d  %s\n", 0, name), 200, "checksum mismatch"},
		{"missing", good, "", 200, "checksum missing"},
		{"invalid checksum", good, "nothex  " + name, 200, "invalid checksum"},
		{"duplicate", good, strings.Repeat(fmt.Sprintf("%x  %s\n", sha256.Sum256(good), name), 2), 200, "duplicate"},
		{"not gzip", []byte("bad"), fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte("bad")), name), 200, "open gzip"},
		{"truncated gzip", good[:len(good)-3], fmt.Sprintf("%x  %s\n", sha256.Sum256(good[:len(good)-3]), name), 200, "extract binary"},
		{"empty binary", compressed(t, nil), fmt.Sprintf("%x  %s\n", sha256.Sum256(compressed(t, nil)), name), 200, "empty"},
		{"http failure", good, "", 404, "404"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != "azssh-updater" {
					t.Error("missing user agent")
				}
				w.WriteHeader(tc.status)
				switch r.URL.Path {
				case "/v1.2.3/SHA256SUMS":
					io.WriteString(w, tc.manifest)
				case "/v1.2.3/" + name:
					w.Write(tc.asset)
				default:
					t.Errorf("unexpected asset path: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			data, err := download(context.Background(), server.Client(), server.URL, "v1.2.3", "linux", "amd64")
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %s", err, tc.want)
				}
			} else if err != nil || string(data) != "verified binary" {
				t.Fatalf("data=%q err=%v", data, err)
			}
		})
	}
}

func TestDownloadRejectsInvalidInputsAndCancellation(t *testing.T) {
	for _, version := range []string{"dev", "v1.2.3/evil", "1.2.3"} {
		if _, err := download(context.Background(), http.DefaultClient, "https://invalid", version, "linux", "amd64"); err == nil {
			t.Fatal("accepted invalid version")
		}
	}
	if _, err := download(context.Background(), http.DefaultClient, "https://invalid", "v1.2.3", "windows", "amd64"); err == nil {
		t.Fatal("accepted unsupported OS")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := download(ctx, http.DefaultClient, "https://invalid", "v1.2.3", "linux", "amd64"); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := readBounded(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("ignored size limit")
	}
}

func TestInstallAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bin", "azssh")
	if err := Install(target, []byte("old")); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := Install(target, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "new" {
		t.Fatalf("data=%s", data)
	}
	previous, _ := io.ReadAll(old)
	if string(previous) != "old" {
		t.Fatal("old inode was modified")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode=%v", info.Mode())
	}
	if err := Install(target, nil); err == nil {
		t.Fatal("accepted empty binary")
	}
	data, _ = os.ReadFile(target)
	if string(data) != "new" {
		t.Fatal("failure changed binary")
	}
	entries, _ := os.ReadDir(filepath.Dir(target))
	if len(entries) != 1 {
		t.Fatal("temporary file leaked")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Install(link, []byte("bad")); err == nil {
		t.Fatal("overwrote symlink")
	}
	if err := Install(dir, []byte("bad")); err == nil {
		t.Fatal("overwrote directory")
	}
}

func TestSudoCommandAndScriptWithoutElevation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "binary with ' quotes ; $chars")
	command, err := SudoCommand(target, []byte("verified"))
	if err != nil {
		t.Fatal(err)
	}
	if command.Args[0] != "sudo" || command.Args[len(command.Args)-1] != target {
		t.Fatalf("args=%q", command.Args)
	}
	data, _ := io.ReadAll(command.Stdin)
	if string(data) != "verified" {
		t.Fatal("stdin is not verified binary")
	}
	// Exercise the actual fixed installation script, but never invoke sudo.
	cmd := exec.Command("/bin/sh", "-c", sudoInstallScript, "azssh-install", target)
	cmd.Stdin = bytes.NewReader(data)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script: %v %s", err, output)
	}
	installed, _ := os.ReadFile(target)
	if string(installed) != "verified" {
		t.Fatal("script installed wrong bytes")
	}
	cmd = exec.Command("/bin/sh", "-c", sudoInstallScript, "azssh-install", target)
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err == nil {
		t.Fatal("script accepted empty input")
	}
	installed, _ = os.ReadFile(target)
	if string(installed) != "verified" {
		t.Fatal("script failure changed target")
	}
	entries, _ := os.ReadDir(filepath.Dir(target))
	if len(entries) != 1 {
		t.Fatal("script temporary file leaked")
	}
	if _, err := SudoCommand("relative", []byte("x")); err == nil {
		t.Fatal("accepted relative destination")
	}
}

func TestLocalPathGuidance(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "azssh")
	if err := Install(target, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !strings.Contains(LocalPathGuidance(target), "PATH resolves") {
		t.Fatal("local binary not detected")
	}
	t.Setenv("PATH", "")
	if !strings.Contains(LocalPathGuidance(target), "Put $HOME/.local/bin first") {
		t.Fatal("missing PATH guidance")
	}
}

func TestInspectInstallationResolvesSymlinkAndCleansProbe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "azssh")
	if err := Install(target, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "azssh-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	installation, err := inspectInstallation(link, dir)
	if err != nil || installation.Current != target || !installation.Writable || installation.Local != filepath.Join(dir, ".local", "bin", "azssh") {
		t.Fatalf("installation=%+v err=%v", installation, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("write probe leaked")
	}
	if os.Geteuid() == 0 {
		return // Root does not exercise directory permission failures.
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755)
	installation, err = inspectInstallation(target, dir)
	if err != nil || installation.Writable {
		t.Fatalf("protected installation=%+v err=%v", installation, err)
	}
}

func TestDownloadPlatformAssets(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(goos+"/"+arch, func(t *testing.T) {
				asset := compressed(t, []byte("binary"))
				name := "azssh_v1.2.3_" + goos + "_" + arch + ".gz"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1.2.3/SHA256SUMS":
						fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(asset), name)
					case "/v1.2.3/" + name:
						w.Write(asset)
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				if _, err := download(context.Background(), server.Client(), server.URL, "v1.2.3", goos, arch); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
