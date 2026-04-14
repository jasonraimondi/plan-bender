package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocs_PrintFlag_EmitsRepoURLOnly(t *testing.T) {
	cmd := NewDocsCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--print"})

	require.NoError(t, cmd.Execute())

	output := strings.TrimSpace(out.String())
	assert.Equal(t, repoURL, output)
}

func TestDocs_FullFlag_EmitsKitchenSinkConfig(t *testing.T) {
	cmd := NewDocsCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--full"})

	require.NoError(t, cmd.Execute())

	output := out.String()
	// Verify it actually contains config keys, not just the repo URL.
	assert.Contains(t, output, "plans_dir:")
	assert.Contains(t, output, "max_points:")
	assert.Contains(t, output, "workflow_states:")
	assert.Contains(t, output, "linear:")
	assert.Contains(t, output, "manage_gitignore:")
	assert.NotContains(t, output, repoURL)
}

func TestDocs_Default_InvokesOpener(t *testing.T) {
	original := docsOpener
	defer func() { docsOpener = original }()

	var openedURL string
	docsOpener = func(url string) error {
		openedURL = url
		return nil
	}

	cmd := NewDocsCmd()
	var out strings.Builder
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())

	assert.Equal(t, repoURL, openedURL)
	assert.Contains(t, out.String(), "Opened "+repoURL)
}

func TestDocs_OpenerFailure_FallsBackToPrint(t *testing.T) {
	original := docsOpener
	defer func() { docsOpener = original }()

	docsOpener = func(url string) error {
		return assert.AnError
	}

	cmd := NewDocsCmd()
	var out strings.Builder
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())

	output := strings.TrimSpace(out.String())
	assert.Equal(t, repoURL, output)
}
