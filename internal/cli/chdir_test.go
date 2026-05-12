package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func chdir(t *testing.T, dir string) {
	t.Helper()

	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(old))
	})
}
