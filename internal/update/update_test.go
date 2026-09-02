package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckResolvesMatchingAsset(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.com/releases/latest" {
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		return responseWithBody(http.StatusOK, `{
			"tag_name":"v1.2.3",
			"body":"Faster updates.\n\nMore detail.",
			"published_at":"2026-03-30T12:00:00Z",
			"assets":[
				{"name":"tide-linux-x86_64.tar.gz","browser_download_url":"https://example.com/download"},
				{"name":"tide-darwin-aarch64.tar.gz","browser_download_url":"https://example.com/other"}
			]
		}`), nil
	})}
	updater := &Updater{
		ReleasesURL: "https://example.com/releases/latest",
		HTTPClient:  client,
		GOOS:        "linux",
		GOARCH:      "amd64",
	}

	result, err := updater.Check("v1.2.2")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Available {
		t.Fatal("expected update to be available")
	}
	if result.Latest.AssetName != "tide-linux-x86_64" {
		t.Fatalf("unexpected asset name: %q", result.Latest.AssetName)
	}
	if result.Latest.DownloadURL != "https://example.com/download" {
		t.Fatalf("unexpected download URL: %q", result.Latest.DownloadURL)
	}
	if result.Latest.Summary != "Faster updates." {
		t.Fatalf("unexpected summary: %q", result.Latest.Summary)
	}
	if result.Latest.PublishedAt.IsZero() {
		t.Fatal("expected published time to be populated")
	}
}

func TestDownloadExtractsBinary(t *testing.T) {
	archive := testArchive(t, "tide-linux-x86_64", "#!/bin/sh\necho tide\n")
	updater := &Updater{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.com/download" {
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/gzip"}},
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	})}}
	asset, err := updater.Download(ReleaseInfo{
		Version:     "v1.2.3",
		AssetName:   "tide-linux-x86_64",
		DownloadURL: "https://example.com/download",
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}

	data, err := os.ReadFile(asset.BinaryPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if !strings.Contains(string(data), "echo tide") {
		t.Fatalf("unexpected binary content: %q", string(data))
	}
}

func TestInstallReplacesExistingBinary(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "tide")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatalf("write current binary: %v", err)
	}
	newBinary := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	updater := New()
	result, err := updater.Install(DownloadedAsset{
		Release:    ReleaseInfo{Version: "v1.2.3"},
		BinaryPath: newBinary,
	}, current)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !result.Restartable {
		t.Fatal("expected install to be restartable")
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("expected current binary to be replaced, got %q", string(data))
	}
}

// When the running binary's dir is not writable, the updater migrates the
// install to ~/.local/bin rather than demanding sudo.
func TestInstallMigratesToUserBinWhenTargetDirNotWritable(t *testing.T) {
	home := t.TempDir()
	restore := stubHome(home)
	defer restore()

	dir := t.TempDir()
	protectedDir := filepath.Join(dir, "protected")
	if err := os.Mkdir(protectedDir, 0o755); err != nil {
		t.Fatalf("create protected dir: %v", err)
	}
	if err := os.Chmod(protectedDir, 0o555); err != nil {
		t.Fatalf("chmod protected dir: %v", err)
	}
	defer os.Chmod(protectedDir, 0o755) //nolint:errcheck

	newBinary := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	result, err := New().Install(DownloadedAsset{
		Release:    ReleaseInfo{Version: "v1.2.3"},
		BinaryPath: newBinary,
	}, filepath.Join(protectedDir, "tide"))
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if result.RequiresManual {
		t.Fatalf("expected migration, not a manual step: %q", result.ManualCommand)
	}
	want := filepath.Join(home, ".local", "bin", "tide")
	if result.ExecutablePath != want {
		t.Fatalf("ExecutablePath = %q, want %q", result.ExecutablePath, want)
	}
	if !result.Restartable {
		t.Fatal("expected migrated install to be restartable")
	}
	if data, _ := os.ReadFile(want); string(data) != "new" {
		t.Fatalf("migrated binary content = %q, want %q", data, "new")
	}
}

// Only when neither the current dir nor ~/.local/bin is usable does it fall
// back to a manual sudo command.
func TestInstallReturnsManualCommandWhenNoWritableTarget(t *testing.T) {
	restoreHome := stubHome(t.TempDir())
	defer restoreHome()
	restoreWritable := dirWritable
	dirWritable = func(string) error { return os.ErrPermission }
	defer func() { dirWritable = restoreWritable }()

	dir := t.TempDir()
	newBinary := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	result, err := New().Install(DownloadedAsset{
		Release:    ReleaseInfo{Version: "v1.2.3"},
		BinaryPath: newBinary,
	}, filepath.Join(dir, "tide"))
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !result.RequiresManual {
		t.Fatal("expected install to require a manual step")
	}
	if !strings.Contains(result.ManualCommand, "sudo install -m 0755") {
		t.Fatalf("unexpected manual command: %q", result.ManualCommand)
	}
}

// After migrating, an older tide earlier on PATH is reported for removal.
func TestInstallReportsShadowedBinary(t *testing.T) {
	home := t.TempDir()
	defer stubHome(home)()

	oldDir := t.TempDir()
	oldExec := filepath.Join(oldDir, "tide")
	if err := os.WriteFile(oldExec, []byte("old"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}
	if err := os.Chmod(oldDir, 0o555); err != nil { // force the migration path
		t.Fatalf("chmod old dir: %v", err)
	}
	defer os.Chmod(oldDir, 0o755) //nolint:errcheck

	userBin := filepath.Join(home, ".local", "bin")
	t.Setenv("PATH", oldDir+string(os.PathListSeparator)+userBin)

	dir := t.TempDir()
	newBinary := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	result, err := New().Install(DownloadedAsset{
		Release:    ReleaseInfo{Version: "v1.2.3"},
		BinaryPath: newBinary,
	}, oldExec)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if result.ShadowedPath != oldExec {
		t.Fatalf("ShadowedPath = %q, want %q", result.ShadowedPath, oldExec)
	}
	if !strings.Contains(result.ShadowedCommand, "sudo rm -f") {
		t.Fatalf("unexpected shadow command: %q", result.ShadowedCommand)
	}
}

func TestInstallTargetPrefersWritableCurrentDir(t *testing.T) {
	dir := t.TempDir()
	got, err := installTarget(filepath.Join(dir, "tide"), false)
	if err != nil {
		t.Fatalf("installTarget: %v", err)
	}
	if got != filepath.Join(dir, "tide") {
		t.Fatalf("installTarget = %q, want the current path", got)
	}
}

func stubHome(home string) func() {
	prev := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	return func() { userHomeDir = prev }
}

func TestIsStableVersion(t *testing.T) {
	if !IsStableVersion("v1.2.3") {
		t.Fatal("expected v1.2.3 to be stable")
	}
	if IsStableVersion("dev") {
		t.Fatal("expected dev to be unstable")
	}
}

func testArchive(t *testing.T, name, content string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name:    name,
		Mode:    0o755,
		Size:    int64(len(content)),
		ModTime: time.Unix(1710000000, 0),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func TestSuggestedManualInstallScriptNonEmpty(t *testing.T) {
	if SuggestedManualInstallScript == "" {
		t.Fatal("SuggestedManualInstallScript is empty")
	}
}

func TestInstallDestinationWritableNoError(t *testing.T) {
	_, err := InstallDestinationWritable()
	if err != nil {
		t.Fatalf("InstallDestinationWritable: %v", err)
	}
}

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func responseWithBody(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
