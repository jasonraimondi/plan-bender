package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncCmd_Structure(t *testing.T) {
	sync := NewSyncCmd()

	linear := findSub(sync, "linear")
	require.NotNil(t, linear, "sync should have a 'linear' subcommand")
	assert.Nil(t, linear.Flags().Lookup("from"), "--from flag was removed; only push/pull subcommands remain")

	push := findSub(linear, "push")
	require.NotNil(t, push, "sync linear should have a 'push' subcommand")
	assert.True(t, push.Runnable())

	pull := findSub(linear, "pull")
	require.NotNil(t, pull, "sync linear should have a 'pull' subcommand")
	assert.True(t, pull.Runnable())

	// push/pull are no longer direct children of sync
	assert.Nil(t, findSub(sync, "push"))
	assert.Nil(t, findSub(sync, "pull"))
}

func TestSyncLinear_BareSlugSuggestsDirection(t *testing.T) {
	sync := NewSyncCmd()
	sync.SetArgs([]string{"linear", "myplan"})
	var stderr bytes.Buffer
	sync.SetErr(&stderr)
	sync.SetOut(&bytes.Buffer{})
	err := sync.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync linear push myplan")
	assert.Contains(t, err.Error(), "sync linear pull myplan")
}

func TestSyncLinear_NoArgsPrintsHelp(t *testing.T) {
	sync := NewSyncCmd()
	sync.SetArgs([]string{"linear"})
	var out bytes.Buffer
	sync.SetOut(&out)
	sync.SetErr(&bytes.Buffer{})
	require.NoError(t, sync.Execute())
	assert.Contains(t, out.String(), "push")
	assert.Contains(t, out.String(), "pull")
}

func TestAgentRoot_SyncLeavesGetSlugCompletion(t *testing.T) {
	root := NewAgentRootCmd("test")

	sync := findSub(root, "sync")
	require.NotNil(t, sync)
	linear := findSub(sync, "linear")
	require.NotNil(t, linear)

	for _, name := range []string{"push", "pull"} {
		leaf := findSub(linear, name)
		require.NotNil(t, leaf, name)
		assert.NotNil(t, leaf.ValidArgsFunction, "%s should have slug completion wired", name)
	}
}

func findSub(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
