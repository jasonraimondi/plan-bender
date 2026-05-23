package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteBugReport(t *testing.T) {
	root := t.TempDir()

	path, err := writeBugReport(root, "v9.9.9", "dispatch foo", errors.New("boom cause"))
	require.NoError(t, err)

	assert.Equal(t, root, filepath.Dir(path))
	assert.True(t, strings.HasPrefix(filepath.Base(path), "pb-error-report-"), "name: %s", path)
	assert.True(t, strings.HasSuffix(path, ".log"), "name: %s", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "pba dispatch foo")
	assert.Contains(t, body, "v9.9.9")
	assert.Contains(t, body, "boom cause")
}
