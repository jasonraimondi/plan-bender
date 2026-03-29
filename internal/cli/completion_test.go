package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugCompletionFunc(t *testing.T) {
	dir := t.TempDir()

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })

	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "not-a-dir.txt"), []byte("x"), 0o644))

	fn := SlugCompletionFunc()
	slugs, directive := fn(nil, nil, "")
	assert.ElementsMatch(t, []string{"alpha", "beta"}, slugs)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestSlugCompletionFunc_SkipsAfterFirstArg(t *testing.T) {
	fn := SlugCompletionFunc()
	slugs, directive := fn(nil, []string{"already-provided"}, "")
	assert.Nil(t, slugs)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
