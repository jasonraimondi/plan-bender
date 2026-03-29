package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addNoopSubcommand adds a trivial subcommand that triggers the full
// PersistentPreRun → Run → PersistentPostRun lifecycle.
func addNoopSubcommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use: "noop",
		Run: func(cmd *cobra.Command, args []string) {},
	})
}

func TestStalenessNotice_PrintsWhenStale(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "1.0.0"
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		return "2.0.0", true, nil
	}

	var stderr bytes.Buffer
	cmd := rootCmd()
	addNoopSubcommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"noop"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "A new version of pb is available")
	assert.Contains(t, stderr.String(), "v1.0.0")
	assert.Contains(t, stderr.String(), "v2.0.0")
	assert.Contains(t, stderr.String(), "pb self-update")
}

func TestStalenessNotice_SuppressedByEnvVar(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "1.0.0"
	called := false
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		called = true
		return "2.0.0", true, nil
	}

	t.Setenv("PB_NO_UPDATE_CHECK", "1")

	var stderr bytes.Buffer
	cmd := rootCmd()
	addNoopSubcommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"noop"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.False(t, called, "should not call CheckForUpdate when env var set")
	assert.NotContains(t, stderr.String(), "new version")
}

func TestStalenessNotice_SuppressedForDevVersion(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "dev"
	called := false
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		called = true
		return "2.0.0", true, nil
	}

	var stderr bytes.Buffer
	cmd := rootCmd()
	addNoopSubcommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"noop"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.False(t, called, "should not call CheckForUpdate for dev version")
	assert.NotContains(t, stderr.String(), "new version")
}

func TestStalenessNotice_NotShownWhenUpToDate(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "1.0.0"
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		return "1.0.0", false, nil
	}

	var stderr bytes.Buffer
	cmd := rootCmd()
	addNoopSubcommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"noop"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.NotContains(t, stderr.String(), "new version")
}

func TestStalenessNotice_NetworkErrorDoesNotBlock(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "1.0.0"
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		return "", false, assert.AnError
	}

	var stderr bytes.Buffer
	cmd := rootCmd()
	addNoopSubcommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"noop"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.NotContains(t, stderr.String(), "new version")
}

func TestStalenessNotice_SuppressedForSelfUpdate(t *testing.T) {
	origVersion := version
	origFn := checkForUpdateFn
	defer func() {
		version = origVersion
		checkForUpdateFn = origFn
	}()

	version = "1.0.0"
	called := false
	checkForUpdateFn = func(currentVersion string) (string, bool, error) {
		called = true
		return "2.0.0", true, nil
	}

	var stderr bytes.Buffer
	cmd := rootCmd()
	cmd.AddCommand(&cobra.Command{
		Use: "self-update",
		Run: func(cmd *cobra.Command, args []string) {},
	})
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"self-update"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.False(t, called, "should not call CheckForUpdate for self-update command")
	assert.NotContains(t, stderr.String(), "new version")
}
