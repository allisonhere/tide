package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShellQuoteRoundTripsLiteralPath(t *testing.T) {
	path := "/tmp/tide $(printf injected) 'quoted'"
	cmd := exec.Command("sh", "-c", "set -- "+shellQuote(path)+"; printf %s \"$1\"")
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("execute quoted argument: %v", err)
	}
	if string(got) != path {
		t.Fatalf("quoted path round-tripped as %q, want %q", got, path)
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	archive := testArchive(t, "tide-linux-x86_64", "new binary")
	updater := &Updater{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/download":
			return responseWithBytes(http.StatusOK, archive), nil
		case "/checksums.txt":
			return responseWithBody(http.StatusOK, strings.Repeat("0", 64)+"  tide-linux-x86_64.tar.gz\n"), nil
		default:
			t.Fatalf("unexpected URL: %s", req.URL)
			return nil, nil
		}
	})}}

	_, err := updater.Download(ReleaseInfo{
		Version: "v1.2.3", AssetName: "tide-linux-x86_64",
		DownloadURL: "https://example.com/download", ChecksumURL: "https://example.com/checksums.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Download error = %v, want checksum mismatch", err)
	}
	entries, readErr := os.ReadDir(tmpRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed download left %d staging directories", len(entries))
	}
}

func TestDownloadRejectsOversizedArchive(t *testing.T) {
	body := bytes.NewReader(make([]byte, maxArchiveBytes+1))
	updater := &Updater{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
	})}}

	_, err := updater.Download(ReleaseInfo{
		Version: "v1.2.3", AssetName: "tide-linux-x86_64", DownloadURL: "https://example.com/download",
	})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Download error = %v, want size-limit error", err)
	}
}

func TestInstallRemovesDownloadedStagingDirectory(t *testing.T) {
	dir := t.TempDir()
	staging := filepath.Join(dir, "staging")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := filepath.Join(staging, "tide-linux-x86_64")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "tide")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := New().Install(DownloadedAsset{
		Release: ReleaseInfo{Version: "v1.2.3"}, BinaryPath: newBinary, TempDir: staging,
	}, current)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging directory still exists after install: %v", err)
	}
	if _, err := os.Stat(current + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup artifact still exists after atomic replacement: %v", err)
	}
}

func TestInstallDefersToPackageManagerForOwnedExecutable(t *testing.T) {
	original := packageUpdateCommand
	packageUpdateCommand = func(path string) (string, bool) {
		if path != "/usr/bin/tide" {
			t.Fatalf("package lookup path = %q, want /usr/bin/tide", path)
		}
		return "yay -S 'tide-bin'", true
	}
	t.Cleanup(func() { packageUpdateCommand = original })

	newBinary := filepath.Join(t.TempDir(), "downloaded")
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := New().Install(DownloadedAsset{
		Release: ReleaseInfo{Version: "v1.3.1"}, BinaryPath: newBinary,
	}, "/usr/bin/tide")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !result.RequiresManual || result.ManualCommand != "yay -S 'tide-bin'" {
		t.Fatalf("Install result = %+v, want package-manager command", result)
	}
}

func TestSuggestedInstallCommandUsesPackageManager(t *testing.T) {
	originalExecutable := executablePath
	originalPackageCommand := packageUpdateCommand
	executablePath = func() (string, error) { return "/usr/bin/tide", nil }
	packageUpdateCommand = func(string) (string, bool) { return "yay -S 'tide-bin'", true }
	t.Cleanup(func() {
		executablePath = originalExecutable
		packageUpdateCommand = originalPackageCommand
	})

	command, err := SuggestedInstallCommand()
	if err != nil {
		t.Fatalf("SuggestedInstallCommand returned error: %v", err)
	}
	if command != "yay -S 'tide-bin'" {
		t.Fatalf("SuggestedInstallCommand = %q, want AUR update command", command)
	}
}

func TestDetectPacmanUpdateCommand(t *testing.T) {
	bin := t.TempDir()
	pacman := filepath.Join(bin, "pacman")
	if err := os.WriteFile(pacman, []byte("#!/bin/sh\nprintf '%s\\n' tide-bin\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "paru"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	command, ok := detectPacmanUpdateCommand("/usr/bin/tide")
	if !ok || command != "paru -S 'tide-bin'" {
		t.Fatalf("detectPacmanUpdateCommand = %q, %v", command, ok)
	}
}

func responseWithBytes(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
}

func checksumLine(name string, body []byte) string {
	return fmt.Sprintf("%x  %s\n", sha256.Sum256(body), name)
}

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
				{"name":"tide-darwin-aarch64.tar.gz","browser_download_url":"https://example.com/other"},
				{"name":"checksums.txt","browser_download_url":"https://example.com/checksums.txt"}
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
	if result.Latest.ChecksumURL != "https://example.com/checksums.txt" {
		t.Fatalf("unexpected checksum URL: %q", result.Latest.ChecksumURL)
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
		switch req.URL.String() {
		case "https://example.com/download":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/gzip"}},
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		case "https://example.com/checksums.txt":
			return responseWithBody(http.StatusOK, checksumLine("tide-linux-x86_64.tar.gz", archive)), nil
		default:
			t.Fatalf("unexpected URL: %s", req.URL)
			return nil, nil
		}
	})}}
	asset, err := updater.Download(ReleaseInfo{
		Version:     "v1.2.3",
		AssetName:   "tide-linux-x86_64",
		DownloadURL: "https://example.com/download",
		ChecksumURL: "https://example.com/checksums.txt",
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(asset.TempDir) })

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
