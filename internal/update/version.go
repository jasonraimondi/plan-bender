package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	cacheTTL    = 24 * time.Hour
	httpTimeout = 3 * time.Second
	cacheFile   = "latest-version.json"
	githubAPI   = "https://api.github.com"
	repoPath    = "/repos/jasonraimondi/plan-bender/releases/latest"
)

type VersionInfo struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func CheckForUpdate(currentVersion string, client *http.Client, force bool) (latest string, isNewer bool, err error) {
	if currentVersion == "dev" {
		return "", false, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", false, fmt.Errorf("determining cache dir: %w", err)
	}

	cachePath := filepath.Join(cacheDir, "plan-bender", cacheFile)

	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}

	return checkForUpdateWith(currentVersion, client, githubAPI, cachePath, force)
}

func checkForUpdateWith(currentVersion string, client *http.Client, baseURL string, cachePath string, force bool) (string, bool, error) {
	if currentVersion == "dev" {
		return "", false, nil
	}

	cached, err := readCache(cachePath)
	if !force && err == nil && time.Since(cached.CheckedAt) < cacheTTL {
		newer, err := isNewerVersion(currentVersion, cached.Version)
		if err != nil {
			return "", false, err
		}
		return cached.Version, newer, nil
	}

	version, err := FetchLatest(client, baseURL)
	if err != nil {
		return "", false, err
	}

	info := VersionInfo{
		Version:   version,
		CheckedAt: time.Now().UTC(),
	}
	if writeErr := writeCache(cachePath, info); writeErr != nil {
		// Non-fatal: cache write failure shouldn't block the check
		_ = writeErr
	}

	newer, err := isNewerVersion(currentVersion, version)
	if err != nil {
		return "", false, err
	}

	return version, newer, nil
}

func FetchLatest(client *http.Client, baseURL string) (string, error) {
	url := baseURL + repoPath

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching latest release: status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decoding release response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func readCache(path string) (VersionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return VersionInfo{}, err
	}

	var info VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return VersionInfo{}, fmt.Errorf("parsing cache: %w", err)
	}

	return info, nil
}

func writeCache(path string, info VersionInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func isNewerVersion(current, latest string) (bool, error) {
	cur, err := parseSemver(current)
	if err != nil {
		return false, fmt.Errorf("parsing current version %q: %w", current, err)
	}

	lat, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("parsing latest version %q: %w", latest, err)
	}

	if lat[0] != cur[0] {
		return lat[0] > cur[0], nil
	}
	if lat[1] != cur[1] {
		return lat[1] > cur[1], nil
	}
	return lat[2] > cur[2], nil
}

func parseSemver(version string) ([3]int, error) {
	version = strings.TrimPrefix(version, "v")
	parts := strings.SplitN(version, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid semver: %q", version)
	}

	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, fmt.Errorf("invalid semver component %q: %w", p, err)
		}
		result[i] = n
	}

	return result, nil
}
