package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckForUpdate_SkipsDevVersion(t *testing.T) {
	latest, isNewer, err := CheckForUpdate("dev", nil)
	require.NoError(t, err)
	assert.Empty(t, latest)
	assert.False(t, isNewer)
}

func TestFetchLatest_ParsesTagName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/jasonraimondi/plan-bender/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.2.3"})
	}))
	defer srv.Close()

	version, err := FetchLatest(srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", version)
}

func TestFetchLatest_HandlesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchLatest(srv.Client(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestCacheReadWrite(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version.json")

	info := VersionInfo{
		Version:   "1.0.0",
		CheckedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := writeCache(cachePath, info)
	require.NoError(t, err)

	got, err := readCache(cachePath)
	require.NoError(t, err)
	assert.Equal(t, info.Version, got.Version)
	assert.Equal(t, info.CheckedAt, got.CheckedAt)
}

func TestReadCache_ReturnsFreshFalseWhenMissing(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "nonexistent.json")

	_, err := readCache(cachePath)
	require.Error(t, err)
}

func TestCacheTTL_FreshCacheReused(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version.json")

	info := VersionInfo{
		Version:   "2.0.0",
		CheckedAt: time.Now().UTC(),
	}
	require.NoError(t, writeCache(cachePath, info))

	fetchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer srv.Close()

	latest, isNewer, err := checkForUpdateWith("1.0.0", srv.Client(), srv.URL, cachePath)
	require.NoError(t, err)
	assert.False(t, fetchCalled, "should not fetch when cache is fresh")
	assert.Equal(t, "2.0.0", latest)
	assert.True(t, isNewer)
}

func TestCacheTTL_ExpiredCacheRefetches(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version.json")

	info := VersionInfo{
		Version:   "1.0.0",
		CheckedAt: time.Now().UTC().Add(-25 * time.Hour),
	}
	require.NoError(t, writeCache(cachePath, info))

	fetchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v3.0.0"})
	}))
	defer srv.Close()

	latest, isNewer, err := checkForUpdateWith("1.0.0", srv.Client(), srv.URL, cachePath)
	require.NoError(t, err)
	assert.True(t, fetchCalled, "should fetch when cache is expired")
	assert.Equal(t, "3.0.0", latest)
	assert.True(t, isNewer)

	// Verify cache was updated
	cached, err := readCache(cachePath)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", cached.Version)
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		expected bool
	}{
		{"same version", "1.0.0", "1.0.0", false},
		{"patch newer", "1.0.0", "1.0.1", true},
		{"minor newer", "1.0.0", "1.1.0", true},
		{"major newer", "1.0.0", "2.0.0", true},
		{"current newer patch", "1.0.2", "1.0.1", false},
		{"current newer minor", "1.1.0", "1.0.9", false},
		{"current newer major", "2.0.0", "1.9.9", false},
		{"with v prefix current", "v1.0.0", "1.0.1", true},
		{"with v prefix latest", "1.0.0", "v1.0.1", true},
		{"with v prefix both", "v1.0.0", "v1.0.1", true},
		{"multi-digit versions", "1.10.0", "1.9.0", false},
		{"multi-digit newer", "1.9.0", "1.10.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isNewerVersion(tt.current, tt.latest)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareSemver_InvalidVersions(t *testing.T) {
	_, err := isNewerVersion("not-a-version", "1.0.0")
	require.Error(t, err)

	_, err = isNewerVersion("1.0.0", "also-not-valid")
	require.Error(t, err)
}

func TestCheckForUpdate_NoCacheDir(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "subdir", "latest-version.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer srv.Close()

	latest, isNewer, err := checkForUpdateWith("1.0.0", srv.Client(), srv.URL, cachePath)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", latest)
	assert.True(t, isNewer)

	// Cache file should have been created
	_, err = os.Stat(cachePath)
	require.NoError(t, err)
}

func TestCheckForUpdate_SameVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.0.0"})
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version.json")

	latest, isNewer, err := checkForUpdateWith("1.0.0", srv.Client(), srv.URL, cachePath)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", latest)
	assert.False(t, isNewer)
}
