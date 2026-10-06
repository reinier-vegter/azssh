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
	"path/filepath"
	"slices"
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

func TestSudoCommandUsesTrustedInstallerAndVerifiedStdin(t *testing.T) {
	command, err := SudoCommand(SystemDestination, SystemDestination, strings.Repeat("a", 64), []byte("verified"))
	if err != nil {
		t.Fatal(err)
	}
	if command.Args[0] != "/usr/bin/sudo" || !slices.Contains(command.Args, "--azssh-install") || !slices.Contains(command.Args, SystemDestination) {
		t.Fatalf("args=%q", command.Args)
	}
	data, _ := io.ReadAll(command.Stdin)
	if string(data) != "verified" {
		t.Fatal("stdin is not verified binary")
	}
	if _, err := SudoCommand("relative", SystemDestination, strings.Repeat("a", 64), []byte("x")); err == nil {
		t.Fatal("accepted untrusted installer")
	}
}

func TestSystemPathGuidance(t *testing.T) {
	t.Setenv("PATH", "")
	if !strings.Contains(SystemPathGuidance(), "Put /usr/local/bin first") {
		t.Fatal("missing PATH guidance")
	}
}

func TestInspectInstallationRejectsNonSystemLocations(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "azssh")
	if err := Install(target, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	installation, err := inspectInstallation(target)
	if err != nil || installation.Current != target || installation.Supported {
		t.Fatalf("installation=%+v err=%v", installation, err)
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
