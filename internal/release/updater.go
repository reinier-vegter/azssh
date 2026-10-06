package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const releaseDownloadURL = "https://github.com/reinier-vegter/azssh/releases/download"
const maxAssetSize = 64 << 20
const maxBinarySize = 128 << 20

// SystemDestination is the supported standalone installation target.
const SystemDestination = "/usr/local/bin/azssh"

// Installation describes the inspected standalone executable.
type Installation struct {
	Current        string
	Writable       bool
	Supported      bool
	ExpectedSHA256 string
}

// InspectInstallation resolves the running executable and accepts only the
// supported system-wide standalone destination.
func InspectInstallation() (Installation, error) {
	if os.Geteuid() == 0 {
		return Installation{}, fmt.Errorf("do not run azssh as root")
	}
	current, err := os.Executable()
	if err != nil {
		return Installation{}, err
	}
	return inspectInstallation(current)
}

func inspectInstallation(current string) (Installation, error) {
	current, err := filepath.EvalSymlinks(current)
	if err != nil {
		return Installation{}, err
	}
	installation := Installation{Current: current, Supported: current == SystemDestination}
	if !installation.Supported {
		return installation, nil
	}
	if err := safeSystemTarget(current); err != nil {
		return Installation{}, err
	}
	digest, err := fileSHA256(current)
	if err != nil {
		return Installation{}, err
	}
	installation.ExpectedSHA256 = digest
	probe, err := os.CreateTemp(filepath.Dir(current), ".azssh-write-check-*")
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		installation.Writable = true
	} else if !os.IsPermission(err) {
		return Installation{}, fmt.Errorf("check installation directory: %w", err)
	}
	return installation, nil
}

func safeSystemTarget(destination string) error {
	if destination != SystemDestination {
		return fmt.Errorf("unsupported installation location %s", destination)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular or symlink destination %s", destination)
	}
	if !ownedByRoot(info) {
		return fmt.Errorf("unsupported installation ownership for %s", destination)
	}
	for dir := filepath.Dir(destination); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() || !ownedByRoot(info) || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("unsafe installation directory %s", dir)
		}
		if dir == "/" {
			break
		}
	}
	return nil
}

func ownedByRoot(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

// Download verifies a release's compressed asset before extracting its binary.
func Download(ctx context.Context, version string) ([]byte, error) {
	artifact, err := Fetch(ctx, version)
	if err != nil {
		return nil, err
	}
	return Verify(artifact)
}

type Artifact struct {
	name            string
	manifest, asset []byte
}

func Fetch(ctx context.Context, version string) (Artifact, error) {
	client := &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return fmt.Errorf("refusing non-HTTPS release redirect")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many release redirects")
		}
		return nil
	}}
	return fetchArtifact(ctx, client, releaseDownloadURL, version, runtime.GOOS, runtime.GOARCH)
}

func download(ctx context.Context, client *http.Client, base, version, goos, arch string) ([]byte, error) {
	artifact, err := fetchArtifact(ctx, client, base, version, goos, arch)
	if err != nil {
		return nil, err
	}
	return Verify(artifact)
}

func fetchArtifact(ctx context.Context, client *http.Client, base, version, goos, arch string) (Artifact, error) {
	if !isReleaseVersion(version) || !strings.HasPrefix(version, "v") {
		return Artifact{}, fmt.Errorf("invalid release version %q", version)
	}
	if (goos != "linux" && goos != "darwin") || (arch != "amd64" && arch != "arm64") {
		return Artifact{}, fmt.Errorf("unsupported platform %s/%s", goos, arch)
	}
	name := "azssh_" + version + "_" + goos + "_" + arch + ".gz"
	url := base + "/" + version + "/"
	manifest, err := fetchBytes(ctx, client, url+"SHA256SUMS", 1<<20)
	if err != nil {
		return Artifact{}, fmt.Errorf("download checksums: %w", err)
	}
	asset, err := fetchBytes(ctx, client, url+name, maxAssetSize)
	if err != nil {
		return Artifact{}, fmt.Errorf("download binary: %w", err)
	}
	return Artifact{name: name, manifest: manifest, asset: asset}, nil
}

func Verify(artifact Artifact) ([]byte, error) {
	var expected []byte
	for _, line := range strings.Split(string(artifact.manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != artifact.name {
			continue
		}
		if expected != nil {
			return nil, fmt.Errorf("duplicate checksum for %s", artifact.name)
		}
		var err error
		expected, err = hex.DecodeString(fields[0])
		if err != nil || len(expected) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum for %s", artifact.name)
		}
	}
	if expected == nil {
		return nil, fmt.Errorf("checksum missing for %s", artifact.name)
	}
	actual := sha256.Sum256(artifact.asset)
	if !bytes.Equal(actual[:], expected) {
		return nil, fmt.Errorf("checksum mismatch for %s", artifact.name)
	}
	reader, err := gzip.NewReader(bytes.NewReader(artifact.asset))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer reader.Close()
	binary, err := readBounded(reader, maxBinarySize)
	if err != nil {
		return nil, fmt.Errorf("extract binary: %w", err)
	}
	if len(binary) == 0 {
		return nil, fmt.Errorf("release binary is empty")
	}
	return binary, nil
}

func fetchBytes(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "azssh-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", response.Status)
	}
	return readBounded(response.Body, limit)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download or extraction exceeds size limit")
	}
	return data, nil
}

// Install atomically replaces a regular executable. It is retained for
// unprivileged test fixtures; updater paths use InstallVerified.
func Install(destination string, binary []byte) error {
	if !filepath.IsAbs(destination) || len(binary) == 0 {
		return fmt.Errorf("installation requires an absolute path and non-empty binary")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if info, err := os.Lstat(destination); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular or symlink destination %s", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return replace(destination, "", sha256Hex(binary), int64(len(binary)), bytes.NewReader(binary), false)
}

// InstallVerified revalidates the original target and verified payload before
// atomically replacing the supported system executable.
func InstallVerified(destination, expectedTarget, payloadDigest string, payloadSize int64, input io.Reader) error {
	if err := safeSystemTarget(destination); err != nil {
		return err
	}
	if actual, err := fileSHA256(destination); err != nil || actual != expectedTarget {
		return fmt.Errorf("installation target changed; review the update again")
	}
	return replace(destination, expectedTarget, payloadDigest, payloadSize, input, true)
}

func replace(destination, expectedTarget, payloadDigest string, payloadSize int64, input io.Reader, system bool) error {
	if payloadSize <= 0 || payloadSize > maxBinarySize || len(payloadDigest) != sha256.Size*2 {
		return fmt.Errorf("invalid verified installation payload")
	}
	if expectedTarget != "" {
		if actual, err := fileSHA256(destination); err != nil || actual != expectedTarget {
			return fmt.Errorf("installation target changed; review the update again")
		}
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".azssh-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(input, payloadSize+1))
	if err != nil {
		return err
	}
	if written != payloadSize {
		return fmt.Errorf("installer received an unexpected payload length")
	}
	if hex.EncodeToString(hash.Sum(nil)) != payloadDigest {
		return fmt.Errorf("installer payload digest mismatch")
	}
	if err = file.Chmod(0755); err != nil {
		return err
	}
	if system {
		if err = file.Chown(0, 0); err != nil {
			return err
		}
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if expectedTarget != "" {
		if actual, err := fileSHA256(destination); err != nil || actual != expectedTarget {
			return fmt.Errorf("installation target changed; review the update again")
		}
	}
	return os.Rename(file.Name(), destination)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sha256Hex(binary []byte) string { sum := sha256.Sum256(binary); return hex.EncodeToString(sum[:]) }

// SudoCommand invokes only an already-installed, administrator-controlled
// executable in its restricted installer mode. Sudo owns terminal input.
func SudoCommand(installerPath, destination, expectedTarget string, binary []byte) (*exec.Cmd, error) {
	if installerPath != SystemDestination || destination != SystemDestination || len(binary) == 0 || !validDigest(expectedTarget) {
		return nil, fmt.Errorf("invalid privileged installation request")
	}
	command := exec.Command("/usr/bin/sudo", "--", installerPath, "--azssh-install", "--destination", destination, "--expected-sha256", expectedTarget, "--payload-sha256", sha256Hex(binary), "--payload-size", fmt.Sprint(len(binary)))
	command.Stdin = bytes.NewReader(binary)
	return command, nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// RunInstaller executes the narrow privileged helper mode.
func RunInstaller(destination, expectedTarget, payloadDigest string, payloadSize int64, input io.Reader) error {
	return InstallVerified(destination, expectedTarget, payloadDigest, payloadSize, input)
}

// SystemPathGuidance reports whether the next shell command selects the update.
func SystemPathGuidance() string {
	resolved, err := exec.LookPath("azssh")
	if err == nil {
		resolved, err = filepath.EvalSymlinks(resolved)
	}
	if err == nil && resolved == SystemDestination {
		return "PATH resolves azssh to /usr/local/bin/azssh."
	}
	return "PATH does not resolve azssh to /usr/local/bin/azssh. Put /usr/local/bin first on PATH and reset your shell command cache (hash -r in Bash)."
}
