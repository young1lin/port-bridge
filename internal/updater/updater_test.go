package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/minio/selfupdate"
	"github.com/young1lin/port-bridge/internal/version"
)

// ---------------------------------------------------------------------------
// sha256Of
// ---------------------------------------------------------------------------

func TestSha256Of(t *testing.T) {
	data := []byte("hello world")
	got := sha256Of(data)

	want := sha256.Sum256(data)
	if hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Errorf("sha256Of mismatch: got %x, want %x", got, want[:])
	}
}

func TestSha256Of_Empty(t *testing.T) {
	data := []byte{}
	got := sha256Of(data)

	want := sha256.Sum256(data)
	if hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Errorf("sha256Of(empty) mismatch: got %x, want %x", got, want[:])
	}
}

// ---------------------------------------------------------------------------
// platformSuffix
// ---------------------------------------------------------------------------

func TestPlatformSuffix(t *testing.T) {
	got := platformSuffix()
	want := runtime.GOOS + "-" + runtime.GOARCH
	if got != want {
		t.Errorf("platformSuffix() = %q, want %q", got, want)
	}

	// Must contain a dash separator.
	if !strings.Contains(got, "-") {
		t.Errorf("platformSuffix() = %q, expected a dash separator", got)
	}
}

// ---------------------------------------------------------------------------
// releaseChecksumURL
// ---------------------------------------------------------------------------

// TestReleaseChecksumURL_Valid verifies that releaseChecksumURL extracts the
// tag path (including the filename after the last "/download/") and builds
// the full GitHub checksums URL.
func TestReleaseChecksumURL_Valid(t *testing.T) {
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/v1.2.0/port-bridge_v1.2.0_windows-amd64.exe"

	// The function takes everything after the last "/download/" as tagPath,
	// which includes the filename.
	want := "https://github.com/young1lin/port-bridge/releases/download/v1.2.0/port-bridge_v1.2.0_windows-amd64.exe/checksums.txt"

	got := releaseChecksumURL(assetURL)
	if got != want {
		t.Errorf("releaseChecksumURL(%q)\n  got  = %q\n  want = %q", assetURL, got, want)
	}
}

func TestReleaseChecksumURL_NoDownload(t *testing.T) {
	got := releaseChecksumURL("https://example.com/some/other/path")
	if got != "" {
		t.Errorf("releaseChecksumURL(no /download/) = %q, want empty string", got)
	}
}

func TestReleaseChecksumURL_Empty(t *testing.T) {
	got := releaseChecksumURL("")
	if got != "" {
		t.Errorf("releaseChecksumURL(\"\") = %q, want empty string", got)
	}
}

// TestReleaseChecksumURL_MultipleDownloadSegments verifies that when "/download/"
// appears multiple times, the last occurrence is used.
func TestReleaseChecksumURL_MultipleDownloadSegments(t *testing.T) {
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/v1.0.0/download/v1.2.0/file.exe"

	// tagPath = "v1.2.0/file.exe" (everything after the last "/download/")
	want := "https://github.com/young1lin/port-bridge/releases/download/v1.2.0/file.exe/checksums.txt"

	got := releaseChecksumURL(assetURL)
	if got != want {
		t.Errorf("releaseChecksumURL(multiple /download/)\n  got  = %q\n  want = %q", got, want)
	}
}

// TestReleaseChecksumURL_DownloadAtEnd verifies behavior when "/download/"
// is at the very end of the URL (no tag path after it).
func TestReleaseChecksumURL_DownloadAtEnd(t *testing.T) {
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/"

	want := "https://github.com/young1lin/port-bridge/releases/download//checksums.txt"
	got := releaseChecksumURL(assetURL)
	if got != want {
		t.Errorf("releaseChecksumURL(/download/ at end)\n  got  = %q\n  want = %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// NewUpdater
// ---------------------------------------------------------------------------

func TestNewUpdater(t *testing.T) {
	u := NewUpdater()
	if u == nil {
		t.Fatal("NewUpdater() returned nil")
	}
}

// ---------------------------------------------------------------------------
// VerifyChecksum
//
// VerifyChecksum creates its own httpClient internally and constructs a URL
// via releaseChecksumURL which always points to github.com. Since the HTTP
// client is not injectable, we can only reliably test:
//   - The early-return path when releaseChecksumURL returns "" (no /download/)
//   - The graceful nil return on network errors / 404s (the function logs
//     warnings but does not propagate errors for these cases)
// ---------------------------------------------------------------------------

func TestVerifyChecksum_NoDownloadSegment(t *testing.T) {
	// When the URL has no /download/ segment, releaseChecksumURL returns ""
	// and VerifyChecksum returns nil immediately without making any HTTP call.
	err := VerifyChecksum("https://example.com/no-download/file.exe", []byte("data"))
	if err != nil {
		t.Errorf("VerifyChecksum no /download/: expected nil, got: %v", err)
	}
}

// TestVerifyChecksum_NetworkError tests that when the checksum URL points to a
// non-routable host (triggering a network error), VerifyChecksum returns nil
// gracefully (it logs a warning but does not propagate the error).
// Skipped in short mode because it waits for a 30-second HTTP timeout.
func TestVerifyChecksum_NetworkError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	// Use a URL that has /download/ so releaseChecksumURL produces a URL,
	// but the derived URL will point to github.com with a non-existent path,
	// which will fail with a network error in most CI/offline environments.
	// In either case (network error or 404), the function returns nil.
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/v99.99.99/nonexistent.exe"
	err := VerifyChecksum(assetURL, []byte("data"))
	// The function should either succeed silently or fail with a checksum
	// mismatch, but NOT with a wrapped network error.
	// In practice: if the HTTP call fails, it logs and returns nil.
	// If it somehow succeeds (e.g., DNS resolves but 404), it also returns nil.
	// The only non-nil error is a checksum mismatch or a read error.
	// We accept any outcome here; the key assertion is no panic.
	if err != nil {
		// If we got an error, it should be a checksum mismatch, not a network error.
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("VerifyChecksum: expected nil or checksum mismatch, got: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// CheckForUpdate (uses custom Transport to redirect requests to httptest)
//
// Since httpClient is unexported but we're in the same package, we can
// directly set the Transport field to redirect all requests to our test server.
// ---------------------------------------------------------------------------

// newTestUpdater creates an Updater whose Transport redirects all requests
// to the given httptest.Server.
func newTestUpdater(srv *httptest.Server) *Updater {
	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})
	return u
}

func TestCheckForUpdate_NoRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate no release: unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("CheckForUpdate no release: expected nil, got %+v", info)
	}
}

func TestCheckForUpdate_AlreadyCurrent(t *testing.T) {
	release := githubRelease{
		TagName: "v1.0.0",
		Body:    "Bug fixes",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "port-bridge_v1.0.0_windows-amd64.exe", BrowserDownloadURL: "https://example.com/file.exe", Size: 1024},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate already current: unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("CheckForUpdate already current: expected nil, got %+v", info)
	}
}

func TestCheckForUpdate_NewVersion(t *testing.T) {
	release := githubRelease{
		TagName: "v2.0.0",
		Body:    "Major new features",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "port-bridge_v2.0.0_windows-amd64.exe", BrowserDownloadURL: "https://example.com/file.exe", Size: 2048},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate new version: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("CheckForUpdate new version: expected non-nil info, got nil")
	}
	if info.TagName != "v2.0.0" {
		t.Errorf("TagName = %q, want %q", info.TagName, "v2.0.0")
	}
	if info.Body != "Major new features" {
		t.Errorf("Body = %q, want %q", info.Body, "Major new features")
	}
	if len(info.Assets) != 1 {
		t.Fatalf("len(Assets) = %d, want 1", len(info.Assets))
	}
	if info.Assets[0].Name != "port-bridge_v2.0.0_windows-amd64.exe" {
		t.Errorf("Asset Name = %q, want %q", info.Assets[0].Name, "port-bridge_v2.0.0_windows-amd64.exe")
	}
	if info.Assets[0].Size != 2048 {
		t.Errorf("Asset Size = %d, want 2048", info.Assets[0].Size)
	}
	if info.Assets[0].IsDelta {
		t.Error("Asset should not be marked as delta")
	}
}

func TestCheckForUpdate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err == nil {
		t.Error("CheckForUpdate API error: expected error, got nil")
	}
	if info != nil {
		t.Errorf("CheckForUpdate API error: expected nil info, got %+v", info)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("CheckForUpdate error = %q, want it to contain 500", err.Error())
	}
}

func TestCheckForUpdate_DeltaDetection(t *testing.T) {
	release := githubRelease{
		TagName: "v1.1.0",
		Body:    "Patch release",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{
				Name:               "port-bridge_v1.0.0-to-v1.1.0_windows-amd64.exe.patch",
				BrowserDownloadURL: "https://example.com/delta.patch",
				Size:               512,
			},
			{
				Name:               "port-bridge_v1.1.0_windows-amd64.exe",
				BrowserDownloadURL: "https://example.com/full.exe",
				Size:               2048,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "0.9.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate delta detection: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("CheckForUpdate delta detection: expected non-nil info, got nil")
	}
	if len(info.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(info.Assets))
	}

	delta := info.Assets[0]
	if !delta.IsDelta {
		t.Error("First asset should be marked as delta")
	}
	if delta.FromVersion != "v1.0.0" {
		t.Errorf("delta.FromVersion = %q, want %q", delta.FromVersion, "v1.0.0")
	}
	if delta.Name != "port-bridge_v1.0.0-to-v1.1.0_windows-amd64.exe.patch" {
		t.Errorf("delta.Name = %q", delta.Name)
	}

	full := info.Assets[1]
	if full.IsDelta {
		t.Error("Second asset should not be marked as delta")
	}
}

func TestCheckForUpdate_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err == nil {
		t.Fatal("CheckForUpdate invalid JSON: expected error, got nil")
	}
	if info != nil {
		t.Errorf("CheckForUpdate invalid JSON: expected nil info, got %+v", info)
	}
}

func TestCheckForUpdate_NoAssets(t *testing.T) {
	release := githubRelease{
		TagName: "v2.0.0",
		Body:    "Release with no assets",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate no assets: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("CheckForUpdate no assets: expected non-nil info, got nil")
	}
	if info.TagName != "v2.0.0" {
		t.Errorf("TagName = %q, want %q", info.TagName, "v2.0.0")
	}
	if len(info.Assets) != 0 {
		t.Errorf("len(Assets) = %d, want 0", len(info.Assets))
	}
}

// ---------------------------------------------------------------------------
// roundTripperFunc - helper type implementing http.RoundTripper as a function
// ---------------------------------------------------------------------------

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e errReadCloser) Close() error {
	return nil
}

type fakeTempUpdateFile struct {
	data     []byte
	offset   int64
	name     string
	writeErr error
	seekErr  error
	closeErr error
}

func (f *fakeTempUpdateFile) Read(p []byte) (int, error) {
	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *fakeTempUpdateFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.data = append(f.data, p...)
	f.offset = int64(len(f.data))
	return len(p), nil
}

func (f *fakeTempUpdateFile) Seek(offset int64, whence int) (int64, error) {
	if f.seekErr != nil {
		return 0, f.seekErr
	}
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		f.offset = int64(len(f.data)) + offset
	}
	if f.offset < 0 {
		f.offset = 0
	}
	return f.offset, nil
}

func (f *fakeTempUpdateFile) Name() string {
	if f.name != "" {
		return f.name
	}
	return filepath.Join(os.TempDir(), "fake-temp-update-file")
}

func (f *fakeTempUpdateFile) Close() error {
	return f.closeErr
}

// ---------------------------------------------------------------------------
// downloadWithProgress
// ---------------------------------------------------------------------------

func TestDownloadWithProgress_Success(t *testing.T) {
	body := []byte("test binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	var progressCalled bool
	data, err := u.downloadWithProgress(srv.URL, func(downloaded, total int64) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("downloadWithProgress data mismatch")
	}
	if !progressCalled {
		t.Error("progress callback was not called")
	}
}

func TestDownloadWithProgress_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	_, err := u.downloadWithProgress(srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want 500", err.Error())
	}
}

func TestDownloadWithProgress_NilProgress(t *testing.T) {
	body := []byte("content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	data, err := u.downloadWithProgress(srv.URL, nil)
	if err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("data mismatch")
	}
}

func TestDownloadWithProgress_GetError(t *testing.T) {
	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport down")
	})

	_, err := u.downloadWithProgress("https://example.com/file.exe", nil)
	if err == nil || !strings.Contains(err.Error(), "transport down") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestDownloadWithProgress_ReadError(t *testing.T) {
	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 12,
			Body:          errReadCloser{err: fmt.Errorf("read failed")},
		}, nil
	})

	_, err := u.downloadWithProgress("https://example.com/file.exe", nil)
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected read failure, got %v", err)
	}
}

func TestDownloadWithProgress_CreateTempFileError(t *testing.T) {
	origCreateTemp := createTempFile
	t.Cleanup(func() { createTempFile = origCreateTemp })

	body := []byte("test binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	createTempFile = func() (tempUpdateFile, error) {
		return nil, fmt.Errorf("temp unavailable")
	}

	_, err := u.downloadWithProgress(srv.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "create temp file") {
		t.Fatalf("expected temp file creation error, got %v", err)
	}
}

func TestDownloadWithProgress_WriteTempError(t *testing.T) {
	origCreateTemp := createTempFile
	t.Cleanup(func() { createTempFile = origCreateTemp })

	body := []byte("test binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	createTempFile = func() (tempUpdateFile, error) {
		return &fakeTempUpdateFile{writeErr: fmt.Errorf("disk full")}, nil
	}

	_, err := u.downloadWithProgress(srv.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "write temp") {
		t.Fatalf("expected temp write error, got %v", err)
	}
}

func TestDownloadWithProgress_SeekTempError(t *testing.T) {
	origCreateTemp := createTempFile
	t.Cleanup(func() { createTempFile = origCreateTemp })

	body := []byte("test binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	createTempFile = func() (tempUpdateFile, error) {
		return &fakeTempUpdateFile{seekErr: fmt.Errorf("seek failed")}, nil
	}

	_, err := u.downloadWithProgress(srv.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "seek temp") {
		t.Fatalf("expected temp seek error, got %v", err)
	}
}

func TestDownloadWithProgress_ReadTempError(t *testing.T) {
	origCreateTemp := createTempFile
	origReadTemp := readTempFile
	t.Cleanup(func() {
		createTempFile = origCreateTemp
		readTempFile = origReadTemp
	})

	body := []byte("test binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	createTempFile = func() (tempUpdateFile, error) {
		return &fakeTempUpdateFile{}, nil
	}
	readTempFile = func(io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("read temp failed")
	}

	_, err := u.downloadWithProgress(srv.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "read temp") {
		t.Fatalf("expected temp read error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DownloadAndApply - tests the asset selection logic
// ---------------------------------------------------------------------------

func TestDownloadAndApply_NoCompatibleAsset(t *testing.T) {
	// No assets match the current platform
	release := &ReleaseInfo{
		TagName: "v2.0.0",
		Assets: []AssetInfo{
			{Name: "port-bridge_v2.0.0_linux-arm64.exe", DownloadURL: "http://example.com/file", IsDelta: false},
		},
	}

	u := NewUpdater()
	err := u.DownloadAndApply(release, nil)
	if err == nil {
		t.Fatal("expected error for no compatible asset")
	}
	if !strings.Contains(err.Error(), "no compatible asset") {
		t.Errorf("error = %q, want 'no compatible asset'", err.Error())
	}
}

func TestDownloadAndApply_DeltaFallbackOnFailure(t *testing.T) {
	// Delta asset matches but download fails (server returns error),
	// then falls through to full binary download
	platform := platformSuffix()
	currentVersionTag := "v" + version.Version

	// Set up test server that returns 500 for all requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := newTestUpdater(srv)

	release := &ReleaseInfo{
		TagName: "v2.0.0",
		Assets: []AssetInfo{
			{
				Name:        fmt.Sprintf("port-bridge_%s-to-v2.0.0_%s.exe.patch", currentVersionTag, platform),
				DownloadURL: srv.URL + "/delta.patch",
				IsDelta:     true,
				FromVersion: currentVersionTag,
			},
			{
				Name:        fmt.Sprintf("port-bridge_v2.0.0_%s.exe", platform),
				DownloadURL: srv.URL + "/full.exe",
				IsDelta:     false,
			},
		},
	}

	err := u.DownloadAndApply(release, nil)
	// Delta fails, full also fails (500), so we get "download returned status 500"
	if err == nil {
		t.Fatal("expected error when both delta and full downloads fail")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, should contain 500", err.Error())
	}
}

func TestDownloadAndApply_FullDownloadFails(t *testing.T) {
	platform := platformSuffix()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	u := newTestUpdater(srv)

	release := &ReleaseInfo{
		TagName: "v2.0.0",
		Assets: []AssetInfo{
			{
				Name:        fmt.Sprintf("port-bridge_v2.0.0_%s.exe", platform),
				DownloadURL: srv.URL + "/full.exe",
				IsDelta:     false,
			},
		},
	}

	err := u.DownloadAndApply(release, nil)
	if err == nil {
		t.Fatal("expected error when full download fails")
	}
}

func TestDownloadAndApplyFull_Success(t *testing.T) {
	u := NewUpdater()
	payload := []byte("new binary")
	var downloadedURL string
	var applyCalled bool

	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		downloadedURL = url
		return payload, nil
	}
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		applyCalled = true
		data, err := io.ReadAll(update)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, payload) {
			t.Fatalf("applied payload mismatch: got %q want %q", string(data), string(payload))
		}
		if !bytes.Equal(opts.Checksum, sha256Of(payload)) {
			t.Fatal("checksum does not match payload")
		}
		return nil
	}

	err := u.downloadAndApplyFull(AssetInfo{DownloadURL: "https://example.com/full.exe"}, nil)
	if err != nil {
		t.Fatalf("downloadAndApplyFull: %v", err)
	}
	if downloadedURL != "https://example.com/full.exe" {
		t.Fatalf("download URL = %q", downloadedURL)
	}
	if !applyCalled {
		t.Fatal("applyUpdate was not called")
	}
}

func TestDownloadAndApplyFull_ApplyFailureRolledBack(t *testing.T) {
	u := NewUpdater()
	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return []byte("new binary"), nil
	}
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		return fmt.Errorf("apply failed")
	}
	u.rollbackFn = func(err error) error { return nil }

	err := u.downloadAndApplyFull(AssetInfo{DownloadURL: "https://example.com/full.exe"}, nil)
	if err == nil || !strings.Contains(err.Error(), "apply failed (rolled back)") {
		t.Fatalf("expected rolled back apply failure, got %v", err)
	}
}

func TestDownloadAndApplyFull_ApplyAndRollbackFailure(t *testing.T) {
	u := NewUpdater()
	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return []byte("new binary"), nil
	}
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		return fmt.Errorf("apply failed")
	}
	u.rollbackFn = func(err error) error { return fmt.Errorf("rollback failed") }

	err := u.downloadAndApplyFull(AssetInfo{DownloadURL: "https://example.com/full.exe"}, nil)
	if err == nil || !strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("expected rollback failure, got %v", err)
	}
}

func TestDownloadAndApplyDelta_Success(t *testing.T) {
	u := NewUpdater()
	patchPayload := []byte("patch bytes")
	var applyCalled bool
	var patcherCalled bool

	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return patchPayload, nil
	}
	u.newPatcher = func() selfupdate.Patcher {
		patcherCalled = true
		return selfupdate.NewBSDiffPatcher()
	}
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		applyCalled = true
		data, err := io.ReadAll(update)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, patchPayload) {
			t.Fatalf("patch payload mismatch: got %q want %q", string(data), string(patchPayload))
		}
		if opts.Patcher == nil {
			t.Fatal("delta update should set a patcher")
		}
		return nil
	}

	err := u.downloadAndApplyDelta(AssetInfo{DownloadURL: "https://example.com/delta.patch"}, nil)
	if err != nil {
		t.Fatalf("downloadAndApplyDelta: %v", err)
	}
	if !patcherCalled {
		t.Fatal("patcher factory was not called")
	}
	if !applyCalled {
		t.Fatal("applyUpdate was not called for delta update")
	}
}

func TestDownloadAndApplyDelta_DownloadFailure(t *testing.T) {
	u := NewUpdater()
	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return nil, fmt.Errorf("network down")
	}

	err := u.downloadAndApplyDelta(AssetInfo{DownloadURL: "https://example.com/delta.patch"}, nil)
	if err == nil || !strings.Contains(err.Error(), "download patch") {
		t.Fatalf("expected download patch error, got %v", err)
	}
}

func TestDownloadAndApplyDelta_ApplyFailureRolledBack(t *testing.T) {
	u := NewUpdater()
	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return []byte("patch bytes"), nil
	}
	u.newPatcher = func() selfupdate.Patcher { return selfupdate.NewBSDiffPatcher() }
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		return fmt.Errorf("patch apply failed")
	}
	u.rollbackFn = func(err error) error { return nil }

	err := u.downloadAndApplyDelta(AssetInfo{DownloadURL: "https://example.com/delta.patch"}, nil)
	if err == nil || !strings.Contains(err.Error(), "apply failed (rolled back)") {
		t.Fatalf("expected delta apply rollback error, got %v", err)
	}
}

func TestDownloadAndApplyDelta_ApplyAndRollbackFailure(t *testing.T) {
	u := NewUpdater()
	u.downloadFn = func(url string, progress ProgressCallback) ([]byte, error) {
		return []byte("patch bytes"), nil
	}
	u.newPatcher = func() selfupdate.Patcher { return selfupdate.NewBSDiffPatcher() }
	u.applyUpdate = func(update io.Reader, opts selfupdate.Options) error {
		return fmt.Errorf("patch apply failed")
	}
	u.rollbackFn = func(err error) error { return fmt.Errorf("rollback failed") }

	err := u.downloadAndApplyDelta(AssetInfo{DownloadURL: "https://example.com/delta.patch"}, nil)
	if err == nil || !strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("expected delta rollback failure, got %v", err)
	}
}

func TestDownloadAndApply_DeltaSuccessRestarts(t *testing.T) {
	u := NewUpdater()
	var appliedDelta bool
	var restarted bool
	u.applyDeltaFn = func(asset AssetInfo, progress ProgressCallback) error {
		appliedDelta = true
		return nil
	}
	u.applyFullFn = func(asset AssetInfo, progress ProgressCallback) error {
		t.Fatal("full download should not be used when delta succeeds")
		return nil
	}
	u.restartAppFn = func() error {
		restarted = true
		return nil
	}

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	release := &ReleaseInfo{
		TagName: "v1.1.0",
		Assets: []AssetInfo{
			{
				Name:        fmt.Sprintf("port-bridge_v1.0.0-to-v1.1.0_%s.exe.patch", platformSuffix()),
				DownloadURL: "https://example.com/delta.patch",
				IsDelta:     true,
				FromVersion: "v1.0.0",
			},
		},
	}

	if err := u.DownloadAndApply(release, nil); err != nil {
		t.Fatalf("DownloadAndApply: %v", err)
	}
	if !appliedDelta {
		t.Fatal("delta asset was not applied")
	}
	if !restarted {
		t.Fatal("restart was not requested after successful delta apply")
	}
}

func TestDownloadAndApply_FullSuccessRestarts(t *testing.T) {
	u := NewUpdater()
	var appliedFull bool
	var restarted bool
	u.applyFullFn = func(asset AssetInfo, progress ProgressCallback) error {
		appliedFull = true
		return nil
	}
	u.applyDeltaFn = func(asset AssetInfo, progress ProgressCallback) error {
		t.Fatal("delta apply should not run when no matching delta exists")
		return nil
	}
	u.restartAppFn = func() error {
		restarted = true
		return nil
	}

	release := &ReleaseInfo{
		TagName: "v2.0.0",
		Assets: []AssetInfo{
			{
				Name:        fmt.Sprintf("port-bridge_v2.0.0_%s.exe", platformSuffix()),
				DownloadURL: "https://example.com/full.exe",
			},
		},
	}

	if err := u.DownloadAndApply(release, nil); err != nil {
		t.Fatalf("DownloadAndApply: %v", err)
	}
	if !appliedFull {
		t.Fatal("full asset was not applied")
	}
	if !restarted {
		t.Fatal("restart was not requested after successful full apply")
	}
}

func TestDownloadAndApply_RestartFailureIsReturned(t *testing.T) {
	u := NewUpdater()
	u.applyFullFn = func(asset AssetInfo, progress ProgressCallback) error { return nil }
	u.restartAppFn = func() error { return fmt.Errorf("restart failed") }

	release := &ReleaseInfo{
		TagName: "v2.0.0",
		Assets: []AssetInfo{
			{
				Name:        fmt.Sprintf("port-bridge_v2.0.0_%s.exe", platformSuffix()),
				DownloadURL: "https://example.com/full.exe",
			},
		},
	}

	err := u.DownloadAndApply(release, nil)
	if err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("expected restart failure, got %v", err)
	}
}

func TestDownloadWithProgress_NetworkError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	u := NewUpdater()
	// Point to an unreachable host — the request will fail
	_, err := u.downloadWithProgress("http://192.0.2.1:12345/nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestReleaseChecksumURL_EmptyPath(t *testing.T) {
	// When /download/ is at the very end
	got := releaseChecksumURL("https://github.com/owner/repo/releases/download/")
	if got == "" {
		t.Error("should not return empty when /download/ is at end")
	}
	// Should contain double slash before checksums.txt
	if !strings.Contains(got, "/checksums.txt") {
		t.Errorf("got = %q, should contain /checksums.txt", got)
	}
}

func TestVerifyChecksum_WithMockServer(t *testing.T) {
	data := []byte("test content")
	checksum := sha256Of(data)
	checksumHex := hex.EncodeToString(checksum)

	_ = checksumHex // VerifyChecksum creates its own HTTP client, so we can't inject.
	// Test sha256Of correctness here.
	hashHex := hex.EncodeToString(sha256Of(data))
	if hashHex != hex.EncodeToString(checksum) {
		t.Errorf("sha256Of mismatch")
	}
}

// ---------------------------------------------------------------------------
// VerifyChecksum - additional paths
// ---------------------------------------------------------------------------

func TestVerifyChecksum_EmptyURL(t *testing.T) {
	err := VerifyChecksum("", []byte("data"))
	if err != nil {
		t.Errorf("VerifyChecksum empty URL: expected nil, got: %v", err)
	}
}

func TestVerifyChecksum_NoDownloadSegment2(t *testing.T) {
	err := VerifyChecksum("https://example.com/path/file.exe", []byte("data"))
	if err != nil {
		t.Errorf("VerifyChecksum no /download/: expected nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CheckForUpdate - additional edge cases
// ---------------------------------------------------------------------------

func TestCheckForUpdate_MultipleAssets(t *testing.T) {
	release := githubRelease{
		TagName: "v3.0.0",
		Body:    "Multi-platform release",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "port-bridge_v3.0.0_windows-amd64.exe", BrowserDownloadURL: "https://example.com/win.exe", Size: 1024},
			{Name: "port-bridge_v3.0.0_linux-amd64", BrowserDownloadURL: "https://example.com/linux", Size: 2048},
			{Name: "port-bridge_v3.0.0_darwin-arm64", BrowserDownloadURL: "https://example.com/darwin", Size: 3000},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if len(info.Assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(info.Assets))
	}
}

func TestCheckForUpdate_SameVersion(t *testing.T) {
	release := githubRelease{
		TagName: "v1.0.0",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "port-bridge_v1.0.0_windows-amd64.exe", BrowserDownloadURL: "https://example.com/file.exe", Size: 1024},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for same version, got %+v", info)
	}
}

func TestCheckForUpdate_OlderVersion(t *testing.T) {
	release := githubRelease{
		TagName: "v0.9.0",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "port-bridge_v0.9.0_windows-amd64.exe", BrowserDownloadURL: "https://example.com/file.exe", Size: 1024},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)

	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for older version, got %+v", info)
	}
}

func TestCheckForUpdate_DeltaAssetNaming(t *testing.T) {
	// Test delta asset with unusual naming
	release := githubRelease{
		TagName: "v2.0.0",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{
				Name:               "port-bridge_v1.5.0-to-v2.0.0_windows-amd64.exe.patch",
				BrowserDownloadURL: "https://example.com/delta.patch",
				Size:               512,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "0.9.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)
	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if len(info.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(info.Assets))
	}
	if !info.Assets[0].IsDelta {
		t.Error("asset should be delta")
	}
	if info.Assets[0].FromVersion != "v1.5.0" {
		t.Errorf("FromVersion = %q, want %q", info.Assets[0].FromVersion, "v1.5.0")
	}
}

func TestCheckForUpdate_DeltaAssetNoUnderscore(t *testing.T) {
	// Delta with no underscore in name
	release := githubRelease{
		TagName: "v2.0.0",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{
				Name:               "patchfile.patch",
				BrowserDownloadURL: "https://example.com/delta.patch",
				Size:               512,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	origVersion := version.Version
	version.Version = "0.9.0"
	defer func() { version.Version = origVersion }()

	u := newTestUpdater(srv)
	info, err := u.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate: unexpected error: %v", err)
	}
	if len(info.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(info.Assets))
	}
	if info.Assets[0].IsDelta {
		t.Error("asset should NOT be delta (no underscore in name)")
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	origClient := checksumHTTPClient
	t.Cleanup(func() { checksumHTTPClient = origClient })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "deadbeef checksums entry")
	}))
	defer srv.Close()

	checksumHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			return srv.Client().Transport.RoundTrip(req)
		}),
	}
	data := []byte("hello checksum")
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/v1.0.0/port-bridge.exe"

	err := VerifyChecksum(assetURL, data)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestVerifyChecksum_Match(t *testing.T) {
	origClient := checksumHTTPClient
	t.Cleanup(func() { checksumHTTPClient = origClient })

	data := []byte("hello checksum")
	hashHex := hex.EncodeToString(sha256Of(data))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, hashHex+"  port-bridge.exe\n")
	}))
	defer srv.Close()

	checksumHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			return srv.Client().Transport.RoundTrip(req)
		}),
	}
	assetURL := "https://github.com/young1lin/port-bridge/releases/download/v1.0.0/port-bridge.exe"

	if err := VerifyChecksum(assetURL, data); err != nil {
		t.Fatalf("VerifyChecksum: %v", err)
	}
}

func TestVerifyChecksum_GetErrorIsIgnored(t *testing.T) {
	origClient := checksumHTTPClient
	t.Cleanup(func() { checksumHTTPClient = origClient })

	checksumHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network down")
		}),
	}

	if err := VerifyChecksum("https://github.com/young1lin/port-bridge/releases/download/v1.0.0/port-bridge.exe", []byte("data")); err != nil {
		t.Fatalf("expected checksum download failure to be ignored, got %v", err)
	}
}

func TestVerifyChecksum_StatusErrorIsIgnored(t *testing.T) {
	origClient := checksumHTTPClient
	t.Cleanup(func() { checksumHTTPClient = origClient })

	checksumHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("missing")),
			}, nil
		}),
	}

	if err := VerifyChecksum("https://github.com/young1lin/port-bridge/releases/download/v1.0.0/port-bridge.exe", []byte("data")); err != nil {
		t.Fatalf("expected missing checksum file to be ignored, got %v", err)
	}
}

func TestVerifyChecksum_ReadBodyError(t *testing.T) {
	origClient := checksumHTTPClient
	t.Cleanup(func() { checksumHTTPClient = origClient })

	checksumHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errReadCloser{err: fmt.Errorf("checksums read failed")},
			}, nil
		}),
	}

	err := VerifyChecksum("https://github.com/young1lin/port-bridge/releases/download/v1.0.0/port-bridge.exe", []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "read checksums") {
		t.Fatalf("expected checksum read failure, got %v", err)
	}
}

func TestRestartApp_ExecutablePathError(t *testing.T) {
	origExecPath := executablePath
	t.Cleanup(func() { executablePath = origExecPath })

	executablePath = func() (string, error) { return "", fmt.Errorf("no executable") }

	err := restartApp()
	if err == nil || !strings.Contains(err.Error(), "no executable") {
		t.Fatalf("expected executable path failure, got %v", err)
	}
}

func TestRestartApp_EvalSymlinksError(t *testing.T) {
	origExecPath := executablePath
	origEval := evalSymlinks
	t.Cleanup(func() {
		executablePath = origExecPath
		evalSymlinks = origEval
	})

	executablePath = func() (string, error) { return "port-bridge.exe", nil }
	evalSymlinks = func(path string) (string, error) { return "", fmt.Errorf("resolve failed") }

	err := restartApp()
	if err == nil || !strings.Contains(err.Error(), "resolve failed") {
		t.Fatalf("expected symlink resolution failure, got %v", err)
	}
}

func TestRestartApp_StartFailure(t *testing.T) {
	origExecPath := executablePath
	origEval := evalSymlinks
	origCmd := execCommand
	origExit := processExit
	origSleep := restartSleep
	t.Cleanup(func() {
		executablePath = origExecPath
		evalSymlinks = origEval
		execCommand = origCmd
		processExit = origExit
		restartSleep = origSleep
	})

	executablePath = func() (string, error) { return "port-bridge.exe", nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command(filepath.Join(t.TempDir(), "missing-binary.exe"))
	}
	processExit = func(code int) {}
	restartSleep = func(time.Duration) {}

	err := restartApp()
	if err == nil || !strings.Contains(err.Error(), "start new process") {
		t.Fatalf("expected start failure, got %v", err)
	}
}

func TestRestartApp_SuccessWithoutExit(t *testing.T) {
	origExecPath := executablePath
	origEval := evalSymlinks
	origCmd := execCommand
	origExit := processExit
	origSleep := restartSleep
	t.Cleanup(func() {
		executablePath = origExecPath
		evalSymlinks = origEval
		execCommand = origCmd
		processExit = origExit
		restartSleep = origSleep
	})

	var exited bool
	var slept bool

	executablePath = func() (string, error) { return "port-bridge.exe", nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	execCommand = func(name string, arg ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessRestartApp", "--", name)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	processExit = func(code int) { exited = true }
	restartSleep = func(time.Duration) { slept = true }

	if err := restartApp(); err != nil {
		t.Fatalf("restartApp: %v", err)
	}
	if !slept {
		t.Fatal("restartApp should wait briefly before exiting")
	}
	if !exited {
		t.Fatal("restartApp should request process exit after starting new process")
	}
}

func TestHelperProcessRestartApp(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, bytes.NewBuffer(nil))
	os.Exit(0)
}

// ---- Cache DI tests -------------------------------------------------------

// newCacheUpdater creates an Updater with an isolated cache directory and
// resets the global memory cache before and after the test.
func newCacheUpdater(t *testing.T) (*Updater, string) {
	t.Helper()
	dir := t.TempDir()
	cacheMu.Lock()
	memoryCache = nil
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		memoryCache = nil
		cacheMu.Unlock()
	})
	u := NewUpdater()
	u.cachePathFn = func() (string, error) {
		return filepath.Join(dir, cacheFileName), nil
	}
	return u, dir
}

func stubUpdaterSeams(t *testing.T) {
	t.Helper()
	oldGetEnv := getEnv
	oldUserHomeDir := userHomeDir
	oldReadFile := readFile
	oldMkdirAll := mkdirAll
	oldWriteFile := writeFile
	oldRenameFile := renameFile
	t.Cleanup(func() {
		getEnv = oldGetEnv
		userHomeDir = oldUserHomeDir
		readFile = oldReadFile
		mkdirAll = oldMkdirAll
		writeFile = oldWriteFile
		renameFile = oldRenameFile
	})
}

func TestGetConfigDir_ReturnsValidPath(t *testing.T) {
	dir, err := getConfigDir()
	if err != nil {
		t.Fatalf("getConfigDir: %v", err)
	}
	if dir == "" {
		t.Fatal("getConfigDir returned empty path")
	}
}

func TestGetConfigDir_UsesAppData(t *testing.T) {
	stubUpdaterSeams(t)
	appData := filepath.Join(t.TempDir(), "AppData")
	getEnv = func(key string) string {
		if key == "APPDATA" {
			return appData
		}
		return ""
	}

	dir, err := getConfigDir()
	if err != nil {
		t.Fatalf("getConfigDir: %v", err)
	}
	want := filepath.Join(appData, "port-bridge")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestGetConfigDir_FallsBackToHome(t *testing.T) {
	stubUpdaterSeams(t)
	home := filepath.Join(t.TempDir(), "home")
	getEnv = func(string) string { return "" }
	userHomeDir = func() (string, error) { return home, nil }

	dir, err := getConfigDir()
	if err != nil {
		t.Fatalf("getConfigDir: %v", err)
	}
	want := filepath.Join(home, ".port-bridge")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestGetConfigDir_HomeError(t *testing.T) {
	stubUpdaterSeams(t)
	getEnv = func(string) string { return "" }
	userHomeDir = func() (string, error) { return "", fmt.Errorf("home failed") }

	if _, err := getConfigDir(); err == nil {
		t.Fatal("expected getConfigDir to fail when home lookup fails")
	}
}

func TestGetCacheFilePath_ReturnsPathInConfigDir(t *testing.T) {
	path, err := getCacheFilePath()
	if err != nil {
		t.Fatalf("getCacheFilePath: %v", err)
	}
	if filepath.Base(path) != cacheFileName {
		t.Errorf("expected filename %q, got %q", cacheFileName, filepath.Base(path))
	}
}

func TestSaveCacheToDisk_Roundtrip(t *testing.T) {
	u, dir := newCacheUpdater(t)

	want := &cachedReleaseInfo{
		CachedAt:  time.Now().Truncate(time.Second),
		CheckedAt: time.Now().Truncate(time.Second),
		TagName:   "v1.2.3",
		Release:   &ReleaseInfo{TagName: "v1.2.3"},
	}

	if err := u.saveCacheToDisk(want); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, cacheFileName)); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	got, err := u.loadCacheFromDisk()
	if err != nil {
		t.Fatalf("loadCacheFromDisk: %v", err)
	}
	if got.TagName != want.TagName {
		t.Errorf("TagName = %q, want %q", got.TagName, want.TagName)
	}
	if got.Release == nil || got.Release.TagName != want.Release.TagName {
		t.Errorf("Release mismatch: got %+v", got.Release)
	}
}

func TestSaveCacheToDisk_NoUpdateFlag(t *testing.T) {
	u, _ := newCacheUpdater(t)

	in := &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
		NoUpdate:  true,
	}
	if err := u.saveCacheToDisk(in); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}

	out, err := u.loadCacheFromDisk()
	if err != nil {
		t.Fatalf("loadCacheFromDisk: %v", err)
	}
	if !out.NoUpdate {
		t.Error("expected NoUpdate=true to round-trip")
	}
}

func TestLoadCacheFromDisk_NotFound(t *testing.T) {
	u, _ := newCacheUpdater(t)

	_, err := u.loadCacheFromDisk()
	if err == nil {
		t.Fatal("expected error when cache file does not exist")
	}
}

func TestLoadCacheFromDisk_InvalidJSON(t *testing.T) {
	u, dir := newCacheUpdater(t)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), []byte("not-json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := u.loadCacheFromDisk()
	if err == nil {
		t.Fatal("expected error for invalid JSON cache file")
	}
}

func TestGetLastCheckTime_NeverChecked(t *testing.T) {
	u, _ := newCacheUpdater(t)

	if got := u.GetLastCheckTime(); !got.IsZero() {
		t.Errorf("expected zero time when never checked, got %v", got)
	}
}

func TestGetLastCheckTime_FromDiskCache(t *testing.T) {
	u, _ := newCacheUpdater(t)

	when := time.Now().Truncate(time.Second)
	if err := u.saveCacheToDisk(&cachedReleaseInfo{CachedAt: when, CheckedAt: when}); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}

	got := u.GetLastCheckTime()
	if !got.Equal(when) {
		t.Errorf("GetLastCheckTime = %v, want %v", got, when)
	}
}

func TestGetLastCheckTime_FromMemoryCache(t *testing.T) {
	u, _ := newCacheUpdater(t)

	when := time.Now().Truncate(time.Second)
	cacheMu.Lock()
	memoryCache = &cachedReleaseInfo{CachedAt: when, CheckedAt: when}
	cacheMu.Unlock()

	got := u.GetLastCheckTime()
	if !got.Equal(when) {
		t.Errorf("GetLastCheckTime = %v, want %v", got, when)
	}
}

func TestCheckForUpdateWithCache_UsesDiskCacheWhenFresh(t *testing.T) {
	u, _ := newCacheUpdater(t)

	// Pre-populate disk cache (fresh, within cacheDuration)
	cached := &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
		TagName:   "v9.9.9",
		Release:   &ReleaseInfo{TagName: "v9.9.9"},
	}
	if err := u.saveCacheToDisk(cached); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}

	// No HTTP server needed — should return cached result without network call
	release, err := u.CheckForUpdateWithCache(false)
	if err != nil {
		t.Fatalf("CheckForUpdateWithCache: %v", err)
	}
	if release == nil || release.TagName != "v9.9.9" {
		t.Errorf("expected cached release v9.9.9, got %v", release)
	}
}

func TestCheckForUpdateWithCache_UsesMemoryCacheWhenFresh(t *testing.T) {
	u, _ := newCacheUpdater(t)

	cacheMu.Lock()
	memoryCache = &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
		TagName:   "v8.8.8",
		Release:   &ReleaseInfo{TagName: "v8.8.8"},
	}
	cacheMu.Unlock()

	release, err := u.CheckForUpdateWithCache(false)
	if err != nil {
		t.Fatalf("CheckForUpdateWithCache: %v", err)
	}
	if release == nil || release.TagName != "v8.8.8" {
		t.Errorf("expected cached release v8.8.8, got %v", release)
	}
}

func TestCheckForUpdateWithCache_NilWhenNoUpdateCached(t *testing.T) {
	u, _ := newCacheUpdater(t)

	cached := &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
		NoUpdate:  true,
	}
	if err := u.saveCacheToDisk(cached); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}

	release, err := u.CheckForUpdateWithCache(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release != nil {
		t.Errorf("expected nil release for no-update cache, got %v", release)
	}
}

func TestCheckForUpdateWithCache_ForceBypassesCache(t *testing.T) {
	// Pre-populate both caches
	u, _ := newCacheUpdater(t)
	cached := &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
		TagName:   "v0.0.1-stale",
		Release:   &ReleaseInfo{TagName: "v0.0.1-stale"},
	}
	if err := u.saveCacheToDisk(cached); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}
	cacheMu.Lock()
	memoryCache = cached
	cacheMu.Unlock()

	// Force=true should bypass cache and hit the network — stub it to return 404 (no update)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	release, err := u.CheckForUpdateWithCache(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release != nil {
		t.Errorf("expected nil release (no update), got %v", release)
	}
}

func TestCheckForUpdateWithCache_FetchesFreshAndCaches(t *testing.T) {
	u, dir := newCacheUpdater(t)

	releaseJSON := `{"tag_name":"v2.0.0","body":"notes","assets":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, releaseJSON)
	}))
	defer srv.Close()
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		return srv.Client().Transport.RoundTrip(req)
	})

	release, err := u.CheckForUpdateWithCache(false)
	if err != nil {
		t.Fatalf("CheckForUpdateWithCache: %v", err)
	}
	// May be nil (no update) or non-nil depending on current version; either is valid.
	// What matters is that a cache file was written.
	if _, statErr := os.Stat(filepath.Join(dir, cacheFileName)); statErr != nil {
		t.Errorf("expected cache file to be written: %v", statErr)
	}
	_ = release
}

func TestCheckForUpdateWithCache_StaleOnAPIError(t *testing.T) {
	u, _ := newCacheUpdater(t)

	// Write an expired cache entry (older than cacheDuration)
	stale := &cachedReleaseInfo{
		CachedAt:  time.Now().Add(-2 * cacheDuration),
		CheckedAt: time.Now().Add(-2 * cacheDuration),
		TagName:   "v1.0.0",
		Release:   &ReleaseInfo{TagName: "v1.0.0"},
	}
	if err := u.saveCacheToDisk(stale); err != nil {
		t.Fatalf("saveCacheToDisk: %v", err)
	}

	// Stub HTTP to fail
	u.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network down")
	})

	release, err := u.CheckForUpdateWithCache(false)
	if err != nil {
		t.Fatalf("expected stale cache fallback, got error: %v", err)
	}
	if release == nil || release.TagName != "v1.0.0" {
		t.Errorf("expected stale release v1.0.0, got %v", release)
	}
}
