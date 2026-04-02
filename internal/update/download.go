package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	githubReleaseBase = "https://github.com/jasonraimondi/plan-bender/releases/download"
	binaryName        = "plan-bender"
	agentBinaryName   = "plan-bender-agent"
	downloadTimeout   = 30 * time.Second
)

func stripVPrefix(version string) string {
	return strings.TrimPrefix(version, "v")
}

func ensureVPrefix(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

// BuildAssetURL constructs the download URL for a platform-specific release archive.
func BuildAssetURL(version, goos, goarch string) string {
	return BuildAssetURLWithBase(version, goos, goarch, githubReleaseBase)
}

func BuildAssetURLWithBase(version, goos, goarch, baseURL string) string {
	v := stripVPrefix(version)
	tag := ensureVPrefix(version)
	filename := fmt.Sprintf("%s_%s_%s_%s.tar.gz", binaryName, v, goos, goarch)
	return fmt.Sprintf("%s/%s/%s", baseURL, tag, filename)
}

// BuildChecksumsURL constructs the download URL for the checksums.txt file.
func BuildChecksumsURL(version string) string {
	return BuildChecksumsURLWithBase(version, githubReleaseBase)
}

func BuildChecksumsURLWithBase(version, baseURL string) string {
	tag := ensureVPrefix(version)
	return fmt.Sprintf("%s/%s/checksums.txt", baseURL, tag)
}

// AssetFilename returns the archive filename for the given version/os/arch.
func AssetFilename(version, goos, goarch string) string {
	v := stripVPrefix(version)
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", binaryName, v, goos, goarch)
}

// VerifyChecksum parses checksums body and verifies that the SHA256 of the file at
// archivePath matches the expected hash for expectedFilename.
// The checksums format is: "{sha256}  {filename}" (two spaces between hash and name).
func VerifyChecksum(archivePath string, checksumsBody []byte, expectedFilename string) error {
	expectedHash, err := findHashInChecksums(checksumsBody, expectedFilename)
	if err != nil {
		return err
	}

	actualHash, err := hashFile(archivePath)
	if err != nil {
		return fmt.Errorf("hashing archive: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", expectedFilename, expectedHash, actualHash)
	}

	return nil
}

func findHashInChecksums(body []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "{hash}  {filename}" (two spaces)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[1]) == filename {
			return strings.TrimSpace(parts[0]), nil
		}
	}
	return "", fmt.Errorf("%s not found in checksums", filename)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ExtractBinaries extracts the plan-bender and plan-bender-agent binaries from
// a gzipped tarball into destDir. Returns paths to both extracted binaries.
func ExtractBinaries(tarballPath, destDir string) (mainBin, agentBin string, err error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return "", "", fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("reading tar entry: %w", err)
		}

		name := filepath.Base(hdr.Name)
		var destPath string
		switch name {
		case binaryName:
			destPath = filepath.Join(destDir, binaryName)
		case agentBinaryName:
			destPath = filepath.Join(destDir, agentBinaryName)
		default:
			continue
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0o755)
		if err != nil {
			return "", "", fmt.Errorf("creating binary file: %w", err)
		}

		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", "", fmt.Errorf("writing binary: %w", err)
		}
		out.Close()

		if name == binaryName {
			mainBin = destPath
		} else {
			agentBin = destPath
		}

		if mainBin != "" && agentBin != "" {
			break
		}
	}

	if mainBin == "" {
		return "", "", fmt.Errorf("plan-bender binary not found in archive")
	}
	if agentBin == "" {
		return "", "", fmt.Errorf("plan-bender-agent binary not found in archive")
	}

	return mainBin, agentBin, nil
}

// DownloadAndVerify downloads the release archive for the given version/os/arch,
// verifies its SHA256 checksum, extracts both binaries, and returns their paths.
// Pass a non-empty baseURL to override the GitHub release base (for testing);
// pass "" to use the default GitHub URL.
func DownloadAndVerify(version, goos, goarch, baseURL string) (mainBin, agentBin string, err error) {
	if baseURL == "" {
		baseURL = githubReleaseBase
	}

	client := &http.Client{Timeout: downloadTimeout}
	tag := ensureVPrefix(version)
	v := stripVPrefix(version)
	filename := AssetFilename(v, goos, goarch)

	// Download checksums.txt
	checksumsURL := fmt.Sprintf("%s/%s/checksums.txt", baseURL, tag)
	checksumsBody, err := httpGet(client, checksumsURL)
	if err != nil {
		return "", "", fmt.Errorf("downloading checksums: %w", err)
	}

	// Download archive to temp file
	archiveURL := fmt.Sprintf("%s/%s/%s", baseURL, tag, filename)
	archiveBytes, err := httpGet(client, archiveURL)
	if err != nil {
		return "", "", fmt.Errorf("downloading archive: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "plan-bender-*.tar.gz")
	if err != nil {
		return "", "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(archiveBytes); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("writing archive: %w", err)
	}
	tmpFile.Close()

	// Verify checksum
	if err := VerifyChecksum(tmpPath, checksumsBody, filename); err != nil {
		os.Remove(tmpPath)
		return "", "", err
	}

	// Extract binaries to temp directory
	extractDir, err := os.MkdirTemp("", "plan-bender-extract-*")
	if err != nil {
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("creating extract dir: %w", err)
	}

	mainBin, agentBin, err = ExtractBinaries(tmpPath, extractDir)
	os.Remove(tmpPath)
	if err != nil {
		os.RemoveAll(extractDir)
		return "", "", err
	}

	return mainBin, agentBin, nil
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
