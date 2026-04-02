package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAssetURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "linux amd64 without v prefix",
			version: "1.2.3",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://github.com/jasonraimondi/plan-bender/releases/download/v1.2.3/plan-bender_1.2.3_linux_amd64.tar.gz",
		},
		{
			name:    "darwin arm64 with v prefix",
			version: "v0.5.0",
			goos:    "darwin",
			goarch:  "arm64",
			want:    "https://github.com/jasonraimondi/plan-bender/releases/download/v0.5.0/plan-bender_0.5.0_darwin_arm64.tar.gz",
		},
		{
			name:    "windows amd64",
			version: "v2.0.0",
			goos:    "windows",
			goarch:  "amd64",
			want:    "https://github.com/jasonraimondi/plan-bender/releases/download/v2.0.0/plan-bender_2.0.0_windows_amd64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAssetURL(tt.version, tt.goos, tt.goarch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildChecksumsURL(t *testing.T) {
	got := BuildChecksumsURL("v1.2.3")
	assert.Equal(t, "https://github.com/jasonraimondi/plan-bender/releases/download/v1.2.3/checksums.txt", got)
}

func TestVerifyChecksum(t *testing.T) {
	content := []byte("hello plan-bender binary")
	hash := sha256.Sum256(content)
	hexHash := fmt.Sprintf("%x", hash)

	tmpFile := filepath.Join(t.TempDir(), "plan-bender_1.0.0_linux_amd64.tar.gz")
	require.NoError(t, os.WriteFile(tmpFile, content, 0o644))

	t.Run("valid checksum passes", func(t *testing.T) {
		checksums := fmt.Sprintf("%s  plan-bender_1.0.0_linux_amd64.tar.gz\nabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  other_file.tar.gz\n", hexHash)
		err := VerifyChecksum(tmpFile, []byte(checksums), "plan-bender_1.0.0_linux_amd64.tar.gz")
		assert.NoError(t, err)
	})

	t.Run("checksum mismatch fails", func(t *testing.T) {
		checksums := "0000000000000000000000000000000000000000000000000000000000000000  plan-bender_1.0.0_linux_amd64.tar.gz\n"
		err := VerifyChecksum(tmpFile, []byte(checksums), "plan-bender_1.0.0_linux_amd64.tar.gz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("filename not found in checksums", func(t *testing.T) {
		checksums := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  some_other_file.tar.gz\n"
		err := VerifyChecksum(tmpFile, []byte(checksums), "plan-bender_1.0.0_linux_amd64.tar.gz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in checksums")
	})
}

func TestExtractBinaries(t *testing.T) {
	t.Run("extracts both binaries from tarball", func(t *testing.T) {
		tarball := createTestTarball(t, map[string][]byte{
			"plan-bender":       []byte("#!/bin/fake-binary"),
			"plan-bender-agent": []byte("#!/bin/fake-agent"),
			"README.md":         []byte("# Plan Bender"),
		})

		dest := t.TempDir()
		mainBin, agentBin, err := ExtractBinaries(tarball, dest)
		require.NoError(t, err)

		assert.Equal(t, filepath.Join(dest, "plan-bender"), mainBin)
		assert.Equal(t, filepath.Join(dest, "plan-bender-agent"), agentBin)

		data, err := os.ReadFile(mainBin)
		require.NoError(t, err)
		assert.Equal(t, "#!/bin/fake-binary", string(data))

		agentData, err := os.ReadFile(agentBin)
		require.NoError(t, err)
		assert.Equal(t, "#!/bin/fake-agent", string(agentData))

		// README should not be extracted
		_, err = os.Stat(filepath.Join(dest, "README.md"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("returns error when main binary not found", func(t *testing.T) {
		tarball := createTestTarball(t, map[string][]byte{
			"plan-bender-agent": []byte("#!/bin/fake-agent"),
		})

		dest := t.TempDir()
		_, _, err := ExtractBinaries(tarball, dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan-bender binary not found")
	})

	t.Run("returns error when agent binary not found", func(t *testing.T) {
		tarball := createTestTarball(t, map[string][]byte{
			"plan-bender": []byte("#!/bin/fake-binary"),
		})

		dest := t.TempDir()
		_, _, err := ExtractBinaries(tarball, dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan-bender-agent binary not found")
	})
}

func TestDownloadAndVerify(t *testing.T) {
	tarballBytes := createTestTarballBytes(t, map[string][]byte{
		"plan-bender":       []byte("#!/bin/fake-plan-bender"),
		"plan-bender-agent": []byte("#!/bin/fake-agent"),
		"README.md":         []byte("# Plan Bender"),
	})

	tarballHash := sha256.Sum256(tarballBytes)
	checksumLine := fmt.Sprintf("%x  plan-bender_1.0.0_darwin_arm64.tar.gz\n", tarballHash)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.0.0/checksums.txt":
			w.Write([]byte(checksumLine))
		case "/releases/download/v1.0.0/plan-bender_1.0.0_darwin_arm64.tar.gz":
			w.Write(tarballBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	mainBin, agentBin, err := DownloadAndVerify("v1.0.0", "darwin", "arm64", srv.URL+"/releases/download")
	require.NoError(t, err)

	data, err := os.ReadFile(mainBin)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/fake-plan-bender", string(data))

	agentData, err := os.ReadFile(agentBin)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/fake-agent", string(agentData))

	// Cleanup
	os.RemoveAll(filepath.Dir(mainBin))
}

func TestDownloadAndVerify_ChecksumMismatch(t *testing.T) {
	tarballBytes := createTestTarballBytes(t, map[string][]byte{
		"plan-bender":       []byte("binary"),
		"plan-bender-agent": []byte("agent"),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.0.0/checksums.txt":
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  plan-bender_1.0.0_linux_amd64.tar.gz\n"))
		case "/releases/download/v1.0.0/plan-bender_1.0.0_linux_amd64.tar.gz":
			w.Write(tarballBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := DownloadAndVerify("v1.0.0", "linux", "amd64", srv.URL+"/releases/download")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestDownloadAndVerify_404(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	_, _, err := DownloadAndVerify("v99.99.99", "linux", "amd64", srv.URL+"/releases/download")
	require.Error(t, err)
}

// helpers

func createTestTarball(t *testing.T, files map[string][]byte) string {
	t.Helper()
	data := createTestTarballBytes(t, files)
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func createTestTarballBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}
