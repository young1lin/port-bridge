package updater

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/minio/selfupdate"
	"github.com/young1lin/port-bridge/internal/version"
)

// ReleaseInfo contains information about a GitHub release.
type ReleaseInfo struct {
	TagName string
	Body    string
	Assets  []AssetInfo
}

// AssetInfo describes a downloadable release asset.
type AssetInfo struct {
	Name        string
	DownloadURL string
	Size        int64
	IsDelta     bool
	FromVersion string
}

// ProgressCallback is called during download to report progress.
type ProgressCallback func(downloaded, total int64)

// githubRelease is the JSON structure returned by the GitHub Releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// cachedReleaseInfo is stored on disk for caching
type cachedReleaseInfo struct {
	Release   *ReleaseInfo `json:"release"`
	CachedAt  time.Time    `json:"cached_at"`
	CheckedAt time.Time    `json:"checked_at"`
	TagName   string       `json:"tag_name"`
	NoUpdate  bool         `json:"no_update"` // true if last check found no update
}

const (
	githubAPIURL  = "https://api.github.com/repos/%s/%s/releases/latest"
	cacheDuration = 1 * time.Hour // Cache valid for 1 hour
	cacheFileName = "update_cache.json"
)

var (
	cacheMu     sync.RWMutex
	memoryCache *cachedReleaseInfo
)

type tempUpdateFile interface {
	io.Reader
	io.Writer
	io.Seeker
	Name() string
	Close() error
}

// Updater handles checking for and applying updates.
type Updater struct {
	httpClient   *http.Client
	applyFullFn  func(asset AssetInfo, progress ProgressCallback) error
	applyDeltaFn func(asset AssetInfo, progress ProgressCallback) error
	restartAppFn func() error
	downloadFn   func(url string, progress ProgressCallback) ([]byte, error)
	applyUpdate  func(update io.Reader, opts selfupdate.Options) error
	rollbackFn   func(err error) error
	newPatcher   func() selfupdate.Patcher
	cachePathFn  func() (string, error) // injectable for tests
}

// NewUpdater creates a new Updater with default settings.
func NewUpdater() *Updater {
	u := &Updater{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	u.applyFullFn = u.downloadAndApplyFull
	u.applyDeltaFn = u.downloadAndApplyDelta
	u.restartAppFn = restartApp
	u.downloadFn = u.downloadWithProgress
	u.applyUpdate = selfupdate.Apply
	u.rollbackFn = selfupdate.RollbackError
	u.newPatcher = selfupdate.NewBSDiffPatcher
	u.cachePathFn = getCacheFilePath
	return u
}

// CheckForUpdate checks GitHub Releases for a newer version.
// Returns a ReleaseInfo if an update is available, or nil if already up to date.
func (u *Updater) CheckForUpdate() (*ReleaseInfo, error) {
	url := fmt.Sprintf(githubAPIURL, version.RepoOwner, version.RepoName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // no releases yet
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if CompareSemver(latestVersion, version.Version) <= 0 {
		return nil, nil // already up to date
	}

	info := &ReleaseInfo{
		TagName: release.TagName,
		Body:    release.Body,
	}

	for _, asset := range release.Assets {
		ai := AssetInfo{
			Name:        asset.Name,
			DownloadURL: asset.BrowserDownloadURL,
			Size:        asset.Size,
		}
		// Detect delta patch: port-bridge_v1.0.0-to-v1.1.0_windows-amd64.exe.patch
		if strings.HasSuffix(asset.Name, ".patch") {
			parts := strings.SplitN(asset.Name, "_", 2)
			if len(parts) == 2 {
				deltaSpec := strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
				// deltaSpec = "v1.0.0-to-v1.1.0_windows-amd64.exe"
				if toIdx := strings.Index(deltaSpec, "-to-"); toIdx > 0 {
					ai.IsDelta = true
					ai.FromVersion = deltaSpec[:toIdx]
				}
			}
		}
		info.Assets = append(info.Assets, ai)
	}

	return info, nil
}

// DownloadAndApply downloads the update and applies it.
// progress is called periodically during the download phase.
func (u *Updater) DownloadAndApply(release *ReleaseInfo, progress ProgressCallback) error {
	platform := platformSuffix()

	// 1. Try delta patch first
	currentVersionTag := "v" + version.Version
	for _, asset := range release.Assets {
		if asset.IsDelta && asset.FromVersion == currentVersionTag && strings.Contains(asset.Name, platform) {
			log.Printf("[UPDATE] Applying delta patch: %s", asset.Name)
			if err := u.applyDeltaFn(asset, progress); err != nil {
				log.Printf("[WARN] Delta patch failed, falling back to full download: %v", err)
				break
			}
			return u.restartAppFn()
		}
	}

	// 2. Fallback to full binary
	for _, asset := range release.Assets {
		if !asset.IsDelta && strings.Contains(asset.Name, platform) {
			log.Printf("[UPDATE] Downloading full binary: %s", asset.Name)
			if err := u.applyFullFn(asset, progress); err != nil {
				return err
			}
			return u.restartAppFn()
		}
	}

	return fmt.Errorf("no compatible asset found for platform %s", platform)
}

// downloadAndApplyFull downloads a full binary and applies the self-update.
func (u *Updater) downloadAndApplyFull(asset AssetInfo, progress ProgressCallback) error {
	data, err := u.downloadFn(asset.DownloadURL, progress)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	opts := selfupdate.Options{
		Checksum: sha256Of(data),
		Hash:     crypto.SHA256,
	}
	if err := u.applyUpdate(bytes.NewReader(data), opts); err != nil {
		if rerr := u.rollbackFn(err); rerr != nil {
			return fmt.Errorf("apply failed and rollback also failed: %v (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("apply failed (rolled back): %w", err)
	}

	return nil
}

// downloadAndApplyDelta downloads a delta patch and applies it to the current binary.
func (u *Updater) downloadAndApplyDelta(asset AssetInfo, progress ProgressCallback) error {
	patchData, err := u.downloadFn(asset.DownloadURL, progress)
	if err != nil {
		return fmt.Errorf("download patch: %w", err)
	}

	opts := selfupdate.Options{
		Patcher: u.newPatcher(),
	}
	if err := u.applyUpdate(bytes.NewReader(patchData), opts); err != nil {
		if rerr := u.rollbackFn(err); rerr != nil {
			return fmt.Errorf("apply failed and rollback also failed: %v (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("apply failed (rolled back): %w", err)
	}

	return nil
}

// downloadWithProgress downloads a URL and calls progress periodically.
func (u *Updater) downloadWithProgress(url string, progress ProgressCallback) ([]byte, error) {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// For large files, use a temp file to avoid memory issues
	tmpFile, err := createTempFile()
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmpFile.Write(buf[:n]); werr != nil {
				return nil, fmt.Errorf("write temp: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, resp.ContentLength)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	// Read back the temp file
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("seek temp: %w", err)
	}

	data, err := readTempFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read temp: %w", err)
	}

	return data, nil
}

// sha256Of computes the SHA-256 hash of data.
func sha256Of(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// platformSuffix returns the OS-arch suffix for the current platform.
func platformSuffix() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// restartApp restarts the current executable.
func restartApp() error {
	exePath, err := executablePath()
	if err != nil {
		return err
	}
	exePath, err = evalSymlinks(exePath)
	if err != nil {
		return err
	}

	cmd := execCommand(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process: %w", err)
	}

	// Give the new process a moment to start
	restartSleep(200 * time.Millisecond)
	processExit(0)
	return nil
}

// VerifyChecksum downloads the checksums file and verifies data against it.
func VerifyChecksum(assetURL string, data []byte) error {
	checksumURL := releaseChecksumURL(assetURL)
	if checksumURL == "" {
		return nil
	}

	resp, err := checksumHTTPClient.Get(checksumURL)
	if err != nil {
		log.Printf("[WARN] Could not download checksums: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[WARN] Checksums file not found (status %d)", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	hashHex := hex.EncodeToString(sha256Of(data))
	if !strings.Contains(string(body), hashHex) {
		return fmt.Errorf("checksum mismatch")
	}

	return nil
}

var (
	execCommand        = exec.Command
	executablePath     = os.Executable
	evalSymlinks       = filepath.EvalSymlinks
	processExit        = os.Exit
	restartSleep       = time.Sleep
	checksumHTTPClient = &http.Client{Timeout: 30 * time.Second}
	createTempFile     = func() (tempUpdateFile, error) { return os.CreateTemp("", "port-bridge-update-*") }
	readTempFile       = func(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
)

// releaseChecksumURL derives the checksums.txt URL from an asset download URL.
func releaseChecksumURL(assetURL string) string {
	idx := strings.LastIndex(assetURL, "/download/")
	if idx < 0 {
		return ""
	}
	tagPath := assetURL[idx+len("/download/"):]
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/checksums.txt",
		version.RepoOwner, version.RepoName, tagPath)
}

// CheckForUpdateWithCache checks for updates with local caching.
// If a cached result exists and is less than cacheDuration old, returns cached result.
// Otherwise, fetches from GitHub API and caches the result.
// Use force = true to bypass cache and always fetch fresh data.
func (u *Updater) CheckForUpdateWithCache(force bool) (*ReleaseInfo, error) {
	cacheMu.RLock()
	// Check memory cache first
	if !force && memoryCache != nil {
		if time.Since(memoryCache.CachedAt) < cacheDuration {
			cacheMu.RUnlock()
			log.Printf("[DEBUG] Using memory-cached update check result")
			if memoryCache.NoUpdate {
				return nil, nil
			}
			return memoryCache.Release, nil
		}
	}
	cacheMu.RUnlock()

	// Check disk cache
	if !force {
		if cached, err := u.loadCacheFromDisk(); err == nil {
			if time.Since(cached.CachedAt) < cacheDuration {
				log.Printf("[DEBUG] Using disk-cached update check result")
				cacheMu.Lock()
				memoryCache = cached
				cacheMu.Unlock()
				if cached.NoUpdate {
					return nil, nil
				}
				return cached.Release, nil
			}
		}
	}

	// Fetch fresh data from GitHub
	log.Printf("[DEBUG] Fetching fresh update check from GitHub API")
	release, err := u.CheckForUpdate()
	if err != nil {
		// On error, try to return stale cache if available
		if cached, cacheErr := u.loadCacheFromDisk(); cacheErr == nil {
			log.Printf("[DEBUG] API failed, returning stale cache: %v", err)
			if cached.NoUpdate {
				return nil, nil
			}
			return cached.Release, nil
		}
		return nil, err
	}

	// Cache the result
	cached := &cachedReleaseInfo{
		CachedAt:  time.Now(),
		CheckedAt: time.Now(),
	}
	if release == nil {
		cached.NoUpdate = true
	} else {
		cached.Release = release
		cached.TagName = release.TagName
	}

	// Update memory cache
	cacheMu.Lock()
	memoryCache = cached
	cacheMu.Unlock()

	// Update disk cache
	if err := u.saveCacheToDisk(cached); err != nil {
		log.Printf("[WARN] Failed to save update cache: %v", err)
	}

	return release, nil
}

// GetLastCheckTime returns when the last update check was performed.
// Returns zero time if never checked.
func (u *Updater) GetLastCheckTime() time.Time {
	cacheMu.RLock()
	if memoryCache != nil {
		defer cacheMu.RUnlock()
		return memoryCache.CheckedAt
	}
	cacheMu.RUnlock()

	cached, err := u.loadCacheFromDisk()
	if err != nil {
		return time.Time{}
	}
	return cached.CheckedAt
}

// getCacheFilePath returns the path to the cache file
func getCacheFilePath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, cacheFileName), nil
}

// getConfigDir returns the application config directory
func getConfigDir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".port-bridge"), nil
	}
	return filepath.Join(appData, "port-bridge"), nil
}

// loadCacheFromDisk loads cached release info from disk
func (u *Updater) loadCacheFromDisk() (*cachedReleaseInfo, error) {
	cachePath, err := u.cachePathFn()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cached cachedReleaseInfo
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

// saveCacheToDisk saves cached release info to disk
func (u *Updater) saveCacheToDisk(cached *cachedReleaseInfo) error {
	cachePath, err := u.cachePathFn()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}

	// Use atomic write pattern
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, cachePath)
}
