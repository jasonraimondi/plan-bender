package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentRootCmd_SuccessExitsClean(t *testing.T) {
	root := NewAgentRootCmd("test")
	root.AddCommand(&cobra.Command{
		Use:  "noop",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})

	root.SetArgs([]string{"noop"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)

	assert.NoError(t, err)
	assert.Empty(t, out.String())
}

func TestAgentRootCmd_ErrorOutputsJSON(t *testing.T) {
	root := NewAgentRootCmd("test")
	root.AddCommand(&cobra.Command{
		Use: "fail",
		RunE: func(cmd *cobra.Command, args []string) error {
			return NewAgentError("something broke", ErrInternal)
		},
	})

	root.SetArgs([]string{"fail"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)

	require.Error(t, err)

	var errResp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &errResp))
	assert.Equal(t, "something broke", errResp.Error)
	assert.Equal(t, string(ErrInternal), errResp.Code)
}

func TestAgentRootCmd_UnknownCommandOutputsJSON(t *testing.T) {
	root := NewAgentRootCmd("test")
	// Add a subcommand so cobra treats unknown args as unknown commands.
	root.AddCommand(&cobra.Command{
		Use:  "valid",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})

	root.SetArgs([]string{"nonexistent"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)

	require.Error(t, err)

	var errResp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &errResp))
	assert.Contains(t, errResp.Error, "nonexistent")
	assert.Equal(t, string(ErrInternal), errResp.Code)
}

func TestAgentRootCmd_PlainErrorGetsInternalCode(t *testing.T) {
	root := NewAgentRootCmd("test")
	root.AddCommand(&cobra.Command{
		Use: "plain-fail",
		RunE: func(cmd *cobra.Command, args []string) error {
			return assert.AnError
		},
	})

	root.SetArgs([]string{"plain-fail"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)

	require.Error(t, err)

	var errResp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &errResp))
	assert.Equal(t, assert.AnError.Error(), errResp.Error)
	assert.Equal(t, string(ErrInternal), errResp.Code)
}

func TestAgentRootCmd_SilencesUsageAndErrors(t *testing.T) {
	root := NewAgentRootCmd("test")

	assert.True(t, root.SilenceErrors)
	assert.True(t, root.SilenceUsage)
}

func TestAgentRootCmd_VersionSet(t *testing.T) {
	root := NewAgentRootCmd("1.2.3")

	assert.Equal(t, "1.2.3", root.Version)
}

func TestAgentError_Unwrap(t *testing.T) {
	err := NewAgentError("test", ErrConfigError)

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, ErrConfigError, agentErr.Code)
	assert.Equal(t, "test", agentErr.Error())
}

func TestErrorCodes_AreDistinct(t *testing.T) {
	codes := []ErrorCode{
		ErrPlanNotFound,
		ErrValidationFailed,
		ErrConfigError,
		ErrInternal,
	}

	seen := make(map[ErrorCode]bool)
	for _, c := range codes {
		assert.False(t, seen[c], "duplicate error code: %s", c)
		seen[c] = true
	}
}
